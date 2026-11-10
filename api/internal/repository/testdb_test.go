package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Test tích hợp cần Postgres có pgvector:
//
//	TEST_DATABASE_URL  kết nối quản trị (trực tiếp) để tạo/xoá database tạm.
//	TEST_POOLER_URL    (tuỳ chọn) đi qua PgBouncer transaction mode, giống
//	                   chuỗi -pooler của Neon; nếu có, code dưới test dùng nó.
//
// Mỗi test chạy trên một database mới tạo rồi xoá, nên chạy song song được.

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL chưa đặt; bỏ qua test tích hợp")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	// Hậu tố ngẫu nhiên: trên macOS UnixNano chỉ chính xác tới micro giây,
	// nên các test song song có thể trùng tên.
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	name := "docinsight_test_" + hex.EncodeToString(suffix)
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database: %v", err)
		}
		admin.Close(ctx)
	})

	base := adminURL
	if p := os.Getenv("TEST_POOLER_URL"); p != "" {
		base = p
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func openTestDB(t *testing.T, dbURL string) *DB {
	t.Helper()
	db, err := NewDB(context.Background(), dbURL, 4)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// newMigratedDB trả DB đã chạy đủ migration thật trong api/migrations.
func newMigratedDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t, testDatabaseURL(t))
	if err := db.Migrate(context.Background(), realMigrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

// vecLiteral tạo literal vector của pgvector, vd. "[0.1,0.2]".
func vecLiteral(vals []float32) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
