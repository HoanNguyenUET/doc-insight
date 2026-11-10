package repository

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"
)

// migrateLockID là khoá advisory để nhiều instance khởi động cùng lúc
// không chạy migration chồng nhau.
const migrateLockID int64 = 0x646f63696e73 // "docins"

// Migrate áp dụng lần lượt các file *.up.sql chưa chạy trong fsys và ghi
// version vào schema_migrations. Cả lần chạy nằm trong MỘT transaction giữ
// pg_advisory_xact_lock, vì:
//   - Neon thường được kết nối qua PgBouncer transaction mode (chuỗi -pooler).
//     Ở đó mỗi câu lệnh ngoài transaction có thể rơi vào backend khác, nên
//     khoá theo session (pg_advisory_lock) vừa không loại trừ được nhau vừa
//     có thể không bao giờ được mở. Khoá theo transaction thì luôn đúng.
//   - Một file lỗi thì cả lần chạy rollback; schema không dừng ở nửa chừng.
func (db *DB) Migrate(ctx context.Context, fsys fs.FS) error {
	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migrations: %w", err)
	}
	// Không làm gì sau Commit; khi có lỗi thì huỷ mọi thay đổi và nhả khoá.
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrateLockID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text        PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	var applied []string
	for _, file := range files {
		version := strings.TrimSuffix(path.Base(file), ".up.sql")

		var done bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&done); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if done {
			continue
		}

		body, err := fs.ReadFile(fsys, file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		// Exec không tham số dùng simple protocol nên chạy được nhiều câu lệnh một lúc.
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		applied = append(applied, version)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	for _, v := range applied {
		slog.InfoContext(ctx, "migration applied", "version", v)
	}
	return nil
}
