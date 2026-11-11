// Package testdb tạo database Postgres tạm cho test tích hợp.
//
//	TEST_DATABASE_URL  kết nối quản trị (trực tiếp) để tạo/xoá database tạm.
//	TEST_POOLER_URL    (tuỳ chọn) đi qua PgBouncer transaction mode, giống
//	                   chuỗi -pooler của Neon; nếu có, code dưới test dùng nó.
//
// Mỗi lần gọi URL tạo một database mới và xoá khi test kết thúc, nên các
// test chạy song song được. Không đặt TEST_DATABASE_URL thì test bị skip.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func URL(t testing.TB) string {
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
