package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/migrations"
)

var realMigrations = migrations.FS

var realVersions = []string{"000001_init", "000002_status_pending", "000003_pages", "000004_embed_model"}

func appliedVersions(t *testing.T, db *DB) []string {
	t.Helper()
	// Lần chạy lỗi rollback cả việc tạo schema_migrations.
	if !tableExists(t, db, "schema_migrations") {
		return nil
	}
	rows, err := db.Pool.Query(context.Background(), `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func tableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var exists bool
	if err := db.Pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
		t.Fatalf("to_regclass(%s): %v", name, err)
	}
	return exists
}

func TestMigrateFreshDatabase(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	if got := appliedVersions(t, db); fmt.Sprint(got) != fmt.Sprint(realVersions) {
		t.Errorf("applied versions = %v, want %v", got, realVersions)
	}
	for _, table := range []string{"documents", "chunks"} {
		if !tableExists(t, db, table) {
			t.Errorf("table %s does not exist", table)
		}
	}

	var embType string
	if err := db.Pool.QueryRow(ctx, `
		SELECT format_type(atttypid, atttypmod) FROM pg_attribute
		WHERE attrelid = 'chunks'::regclass AND attname = 'embedding'`).Scan(&embType); err != nil {
		t.Fatalf("embedding type: %v", err)
	}
	if embType != "vector(768)" {
		t.Errorf("chunks.embedding type = %q, want vector(768)", embType)
	}

	var indexDef string
	if err := db.Pool.QueryRow(ctx,
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'chunks_embedding_ivfflat_idx'`).Scan(&indexDef); err != nil {
		t.Fatalf("embedding index: %v", err)
	}
	for _, want := range []string{"ivfflat", "vector_cosine_ops", "lists='100'"} {
		if !strings.Contains(indexDef, want) {
			t.Errorf("index def %q missing %q", indexDef, want)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)

	if err := db.Migrate(context.Background(), realMigrations); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if got := appliedVersions(t, db); fmt.Sprint(got) != fmt.Sprint(realVersions) {
		t.Errorf("applied versions after rerun = %v, want %v", got, realVersions)
	}
}

// Nhiều instance khởi động cùng lúc (deploy, scale) đều gọi Migrate:
// tất cả phải thành công và migration chỉ chạy đúng một lần.
func TestMigrateConcurrentInstances(t *testing.T) {
	t.Parallel()
	dbURL := testDatabaseURL(t)

	const instances = 8
	dbs := make([]*DB, instances)
	for i := range dbs {
		dbs[i] = openTestDB(t, dbURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := make(chan struct{})
	errs := make([]error, instances)
	var wg sync.WaitGroup
	for i, db := range dbs {
		wg.Add(1)
		go func(i int, db *DB) {
			defer wg.Done()
			<-start
			errs[i] = db.Migrate(ctx, realMigrations)
		}(i, db)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("instance %d: Migrate: %v", i, err)
		}
	}
	if got := appliedVersions(t, dbs[0]); fmt.Sprint(got) != fmt.Sprint(realVersions) {
		t.Errorf("applied versions = %v, want %v (each exactly once)", got, realVersions)
	}
}

// Một lần chạy migration là tất cả hoặc không: nếu một file lỗi thì
// không file nào trong lần đó được áp dụng, và lần sau chạy lại được.
func TestMigrateFailureIsAllOrNothing(t *testing.T) {
	t.Parallel()
	db := openTestDB(t, testDatabaseURL(t))
	ctx := context.Background()

	broken := fstest.MapFS{
		"000001_a.up.sql": {Data: []byte(`CREATE TABLE a (id int);`)},
		"000002_b.up.sql": {Data: []byte(`CREATE TABLE b (id int); SELECT 1/0;`)},
	}
	err := db.Migrate(ctx, broken)
	if err == nil {
		t.Fatal("Migrate with broken file: want error, got nil")
	}
	if !strings.Contains(err.Error(), "000002_b") {
		t.Errorf("error %q does not name the failing migration", err)
	}
	for _, table := range []string{"a", "b"} {
		if tableExists(t, db, table) {
			t.Errorf("table %s exists after failed run", table)
		}
	}
	if got := appliedVersions(t, db); len(got) != 0 {
		t.Errorf("applied versions after failed run = %v, want none", got)
	}

	fixed := fstest.MapFS{
		"000001_a.up.sql": broken["000001_a.up.sql"],
		"000002_b.up.sql": {Data: []byte(`CREATE TABLE b (id int);`)},
	}
	if err := db.Migrate(ctx, fixed); err != nil {
		t.Fatalf("Migrate after fix: %v", err)
	}
	if got := appliedVersions(t, db); len(got) != 2 {
		t.Errorf("applied versions after fix = %v, want 2", got)
	}
}

func TestMigrateAppliesInVersionOrder(t *testing.T) {
	t.Parallel()
	db := openTestDB(t, testDatabaseURL(t))

	// 000002 phụ thuộc 000001; nếu chạy sai thứ tự sẽ lỗi.
	fsys := fstest.MapFS{
		"000002_add_col.up.sql":   {Data: []byte(`ALTER TABLE a ADD COLUMN name text;`)},
		"000001_create.up.sql":    {Data: []byte(`CREATE TABLE a (id int);`)},
		"000001_create.down.sql":  {Data: []byte(`DROP TABLE a;`)},
		"000002_add_col.down.sql": {Data: []byte(`ALTER TABLE a DROP COLUMN name;`)},
	}
	if err := db.Migrate(context.Background(), fsys); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got := appliedVersions(t, db); len(got) != 2 || got[0] != "000001_create" || got[1] != "000002_add_col" {
		t.Errorf("applied versions = %v (down files must be ignored)", got)
	}
}
