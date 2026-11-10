// Package repository là nơi duy nhất chứa SQL và truy cập PostgreSQL.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func NewDB(ctx context.Context, databaseURL string, maxConns int32) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = maxConns
	// Neon tự tắt compute khi rảnh; đóng kết nối nhàn rỗi để không giữ kết nối chết.
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}

	// Neon có thể mất vài giây để khởi động compute từ trạng thái ngủ.
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) Ping(ctx context.Context) error {
	if err := db.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

func (db *DB) Close() {
	db.Pool.Close()
}
