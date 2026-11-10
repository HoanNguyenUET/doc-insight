package repository

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// migrateLockID là khoá advisory để nhiều instance khởi động cùng lúc
// không chạy migration chồng nhau.
const migrateLockID int64 = 0x646f63696e73 // "docins"

// Migrate áp dụng lần lượt các file *.up.sql chưa chạy trong fsys,
// mỗi file trong một transaction, và ghi version vào schema_migrations.
func (db *DB) Migrate(ctx context.Context, fsys fs.FS) error {
	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)

	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLockID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		// Dùng context riêng để vẫn mở khoá được khi ctx đã bị huỷ.
		if _, err := conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrateLockID); err != nil {
			slog.Error("advisory unlock", "err", err)
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text        PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, file := range files {
		version := strings.TrimSuffix(path.Base(file), ".up.sql")

		var applied bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}

		body, err := fs.ReadFile(fsys, file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		if err := applyMigration(ctx, conn.Conn(), version, string(body)); err != nil {
			return err
		}
		slog.Info("migration applied", "version", version)
	}
	return nil
}

func applyMigration(ctx context.Context, conn *pgx.Conn, version, body string) (err error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback migration %s: %w", version, rbErr))
			}
		}
	}()

	// Exec không tham số dùng simple protocol nên chạy được nhiều câu lệnh một lúc.
	if _, err := tx.Exec(ctx, body); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}
