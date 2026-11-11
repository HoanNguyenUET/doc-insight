package repository

import (
	"context"
	"testing"

	"github.com/HoanNguyenUET/doc-insight/api/internal/testdb"
)

// Test tích hợp cần Postgres có pgvector; xem package testdb.

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	return testdb.URL(t)
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
