package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func insertDocument(t *testing.T, db *DB) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(context.Background(),
		`INSERT INTO documents (filename, size_bytes) VALUES ('a.pdf', 1024) RETURNING id`,
	).Scan(&id); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	return id
}

func embedding(dim int, at int) string {
	v := make([]float32, dim)
	v[at%dim] = 1
	return vectorLiteral(v)
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const insertChunkSQL = `
	INSERT INTO chunks (document_id, chunk_index, content, page_number, char_start, char_end, embedding)
	VALUES ($1, $2, 'nội dung', $3, $4, $5, $6::vector)`

func TestChunkConstraints(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	doc := insertDocument(t, db)

	if _, err := db.Pool.Exec(ctx, insertChunkSQL, doc, 0, 7, 0, 480, embedding(768, 0)); err != nil {
		t.Fatalf("insert valid chunk: %v", err)
	}

	tests := []struct {
		name     string
		args     []any
		wantCode string // mã SQLSTATE; "22" = lớp data_exception
	}{
		{"embedding NULL until embedded", []any{doc, 1, 7, 0, 10, nil}, ""},
		{"duplicate chunk_index", []any{doc, 0, 7, 0, 10, nil}, "23505"},
		{"char_end equals char_start", []any{doc, 2, 7, 10, 10, nil}, "23514"},
		{"negative char_start", []any{doc, 3, 7, -1, 10, nil}, "23514"},
		{"page_number zero", []any{doc, 4, 0, 0, 10, nil}, "23514"},
		{"negative chunk_index", []any{doc, -1, 1, 0, 10, nil}, "23514"},
		{"unknown document", []any{"00000000-0000-0000-0000-000000000000", 0, 1, 0, 10, nil}, "23503"},
		{"embedding wrong dimension", []any{doc, 5, 1, 0, 10, embedding(3, 0)}, "22"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Pool.Exec(ctx, insertChunkSQL, tt.args...)
			code := pgCode(err)
			switch {
			case tt.wantCode == "" && err != nil:
				t.Errorf("want success, got %v", err)
			case tt.wantCode != "" && !strings.HasPrefix(code, tt.wantCode):
				t.Errorf("want SQLSTATE %s, got %q (err %v)", tt.wantCode, code, err)
			}
		})
	}
}

func TestDocumentConstraints(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	var status string
	var progress, chunkCount int
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO documents (filename, size_bytes) VALUES ('a.pdf', 1) RETURNING status, progress, chunk_count`,
	).Scan(&status, &progress, &chunkCount); err != nil {
		t.Fatalf("insert with defaults: %v", err)
	}
	if status != "pending" || progress != 0 || chunkCount != 0 {
		t.Errorf("defaults = (%s, %d, %d), want (pending, 0, 0)", status, progress, chunkCount)
	}

	tests := []struct {
		name string
		sql  string
	}{
		{"unknown status", `INSERT INTO documents (filename, size_bytes, status) VALUES ('a.pdf', 1, 'done')`},
		{"progress over 100", `INSERT INTO documents (filename, size_bytes, progress) VALUES ('a.pdf', 1, 101)`},
		{"negative size", `INSERT INTO documents (filename, size_bytes) VALUES ('a.pdf', -1)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Pool.Exec(ctx, tt.sql)
			if code := pgCode(err); code != "23514" {
				t.Errorf("want check violation 23514, got %q (err %v)", code, err)
			}
		})
	}
}

func TestDeleteDocumentCascadesChunks(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	doc := insertDocument(t, db)
	for i := 0; i < 3; i++ {
		if _, err := db.Pool.Exec(ctx, insertChunkSQL, doc, i, 1, 0, 10, nil); err != nil {
			t.Fatalf("insert chunk %d: %v", i, err)
		}
	}

	if _, err := db.Pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, doc); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE document_id = $1`, doc).Scan(&n); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if n != 0 {
		t.Errorf("%d chunks left after deleting document, want 0", n)
	}
}

// Truy vấn tìm kiếm dùng toán tử cosine (<=>) phải đi qua index ivfflat.
// Nếu ai đó đổi opclass (vd. sang vector_l2_ops) thì index sẽ bị bỏ qua
// và tìm kiếm âm thầm thành quét toàn bảng.
func TestCosineSearchUsesIvfflatIndex(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	doc := insertDocument(t, db)
	for i := 0; i < 50; i++ {
		if _, err := db.Pool.Exec(ctx, insertChunkSQL, doc, i, 1, 0, 10, embedding(768, i)); err != nil {
			t.Fatalf("insert chunk %d: %v", i, err)
		}
	}

	var plan []string
	err := pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		// Bảng nhỏ thì planner chọn seq scan; tắt đi để kiểm tra index có dùng được không.
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`EXPLAIN SELECT id FROM chunks ORDER BY embedding <=> $1::vector LIMIT 5`, embedding(768, 7))
		if err != nil {
			return err
		}
		plan, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if joined := strings.Join(plan, "\n"); !strings.Contains(joined, "chunks_embedding_ivfflat_idx") {
		t.Errorf("cosine search does not use the ivfflat index:\n%s", joined)
	}
}
