package repository

import (
	"context"
	"errors"
	"testing"
)

func chunkCount(t *testing.T, db *DB, docID string) int {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM chunks WHERE document_id = $1`, docID).Scan(&n); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	return n
}

func mustGet(t *testing.T, db *DB, id string) Document {
	t.Helper()
	d, err := db.GetDocument(context.Background(), id)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	return d
}

func TestCreateGetDocument(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	d, err := db.CreateDocument(ctx, "báo cáo.pdf", 2048)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if d.ID == "" || d.Filename != "báo cáo.pdf" || d.SizeBytes != 2048 || d.Status != StatusPending || d.PageCount != nil || d.Error != nil {
		t.Errorf("created = %+v", d)
	}
	if got := mustGet(t, db, d.ID); got.ID != d.ID || got.Status != StatusPending {
		t.Errorf("GetDocument = %+v", got)
	}

	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid", "", "'; DROP TABLE documents; --"} {
		if _, err := db.GetDocument(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetDocument(%q) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestListDocumentsNewestFirst(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	var ids []string
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		d, err := db.CreateDocument(ctx, name, 1)
		if err != nil {
			t.Fatalf("CreateDocument: %v", err)
		}
		ids = append(ids, d.ID)
	}
	// created_at mặc định là now() của transaction, có thể trùng nhau; đặt hẳn giá trị.
	for i, id := range ids {
		if _, err := db.Pool.Exec(ctx, `UPDATE documents SET created_at = now() + make_interval(secs => $2) WHERE id = $1`, id, i); err != nil {
			t.Fatal(err)
		}
	}

	docs, err := db.ListDocuments(ctx, 10)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 3 || docs[0].ID != ids[2] || docs[1].ID != ids[1] || docs[2].ID != ids[0] {
		t.Errorf("order = %v, want newest first", docs)
	}
	if docs, _ := db.ListDocuments(ctx, 2); len(docs) != 2 {
		t.Errorf("limit 2 returned %d", len(docs))
	}
}

func TestDocumentLifecycle(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	d, _ := db.CreateDocument(ctx, "a.pdf", 1)

	if err := db.FailDocument(ctx, d.ID, "lần trước lỗi"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDocumentStage(ctx, d.ID, StatusExtracting, 10); err != nil {
		t.Fatalf("SetDocumentStage: %v", err)
	}
	got := mustGet(t, db, d.ID)
	if got.Status != StatusExtracting || got.Progress != 10 || got.Error != nil {
		t.Errorf("after extracting = %+v (error of previous run must be cleared)", got)
	}
	if err := db.SetDocumentStage(ctx, d.ID, StatusChunking, 40); err != nil {
		t.Fatal(err)
	}

	emb := make([]float32, 768)
	emb[3] = 0.5
	chunks := []NewChunk{
		{Index: 0, Content: "Đoạn một.", PageNumber: 1, CharStart: 0, CharEnd: 9, TokenCount: 3},
		{Index: 1, Content: "Đoạn hai.", PageNumber: 2, CharStart: 5, CharEnd: 14, TokenCount: 3, Embedding: emb},
	}
	if err := db.CompleteDocument(ctx, d.ID, "", blankPages(2), chunks); err != nil {
		t.Fatalf("CompleteDocument: %v", err)
	}
	got = mustGet(t, db, d.ID)
	if got.Status != StatusReady || got.Progress != 100 || got.ChunkCount != 2 || got.PageCount == nil || *got.PageCount != 2 || got.ProcessedAt == nil {
		t.Errorf("after complete = %+v", got)
	}

	rows, err := db.Pool.Query(ctx, `SELECT chunk_index, content, page_number, char_start, char_end, token_count, embedding IS NULL, (embedding::real[])[4]
		FROM chunks WHERE document_id = $1 ORDER BY chunk_index`, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		idx, page, cs, ce, tok int
		content                string
		embNull                bool
		emb3                   *float32
	}
	var got2 []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.idx, &r.content, &r.page, &r.cs, &r.ce, &r.tok, &r.embNull, &r.emb3); err != nil {
			t.Fatal(err)
		}
		got2 = append(got2, r)
	}
	if len(got2) != 2 ||
		got2[0].content != "Đoạn một." || got2[0].page != 1 || got2[0].cs != 0 || got2[0].ce != 9 || !got2[0].embNull ||
		got2[1].page != 2 || got2[1].cs != 5 || got2[1].ce != 14 || got2[1].embNull || got2[1].emb3 == nil || *got2[1].emb3 != 0.5 {
		t.Errorf("stored chunks = %+v", got2)
	}

	// Chạy lại thay thế, không cộng dồn.
	if err := db.CompleteDocument(ctx, d.ID, "", blankPages(1), chunks[:1]); err != nil {
		t.Fatalf("CompleteDocument rerun: %v", err)
	}
	if n := chunkCount(t, db, d.ID); n != 1 || mustGet(t, db, d.ID).ChunkCount != 1 {
		t.Errorf("after rerun: %d chunks, want 1", n)
	}
}

// Một chunk lỗi thì không chunk nào được lưu và trạng thái không thành ready.
func TestCompleteDocumentIsAllOrNothing(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	d, _ := db.CreateDocument(ctx, "a.pdf", 1)
	_ = db.SetDocumentStage(ctx, d.ID, StatusEmbedding, 70)

	chunks := make([]NewChunk, 120)
	for i := range chunks {
		chunks[i] = NewChunk{Index: i, Content: "x", PageNumber: 1, CharStart: i, CharEnd: i + 1}
	}
	chunks[119].CharEnd = chunks[119].CharStart // vi phạm chunks_char_range_check

	if err := db.CompleteDocument(ctx, d.ID, "", blankPages(1), chunks); err == nil {
		t.Fatal("want error, got nil")
	}
	if n := chunkCount(t, db, d.ID); n != 0 {
		t.Errorf("%d chunks persisted after failed transaction", n)
	}
	if got := mustGet(t, db, d.ID); got.Status != StatusEmbedding || got.ChunkCount != 0 {
		t.Errorf("document changed by failed transaction: %+v", got)
	}
}

func TestStageUpdatesOnMissingDocument(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	missing := "00000000-0000-0000-0000-000000000000"

	if err := db.SetDocumentStage(ctx, missing, StatusExtracting, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetDocumentStage err = %v", err)
	}
	if err := db.FailDocument(ctx, missing, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FailDocument err = %v", err)
	}
	if err := db.CompleteDocument(ctx, missing, "", blankPages(1), nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("CompleteDocument err = %v", err)
	}
}

func TestDeleteDocument(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()
	d, _ := db.CreateDocument(ctx, "a.pdf", 1)
	if err := db.CompleteDocument(ctx, d.ID, "", blankPages(1), []NewChunk{{Content: "x", PageNumber: 1, CharEnd: 1}}); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteDocument(ctx, d.ID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if n := chunkCount(t, db, d.ID); n != 0 {
		t.Errorf("%d chunks left", n)
	}
	if err := db.DeleteDocument(ctx, d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete err = %v, want ErrNotFound", err)
	}
	if err := db.DeleteDocument(ctx, "bad"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete bad id err = %v, want ErrNotFound", err)
	}
}

func TestFailUnfinishedDocuments(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	// Mỗi tài liệu được đưa tới một trạng thái, như khi tiến trình chết đột ngột.
	tests := []struct {
		status     string
		wantStatus string
		wantError  string
	}{
		{StatusPending, StatusFailed, "dừng"},
		{StatusExtracting, StatusFailed, "dừng"},
		{StatusChunking, StatusFailed, "dừng"},
		{StatusEmbedding, StatusFailed, "dừng"},
		{StatusReady, StatusReady, ""},
		{StatusFailed, StatusFailed, "lỗi cũ"},
	}
	ids := make([]string, len(tests))
	for i, tc := range tests {
		d, err := db.CreateDocument(ctx, tc.status+".pdf", 1)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = d.ID
		switch tc.status {
		case StatusPending:
		case StatusReady:
			err = db.CompleteDocument(ctx, d.ID, "", blankPages(1), nil)
		case StatusFailed:
			err = db.FailDocument(ctx, d.ID, "lỗi cũ")
		default:
			err = db.SetDocumentStage(ctx, d.ID, tc.status, 50)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	n, err := db.FailUnfinishedDocuments(ctx, "dừng")
	if err != nil {
		t.Fatalf("FailUnfinishedDocuments: %v", err)
	}
	if n != 4 {
		t.Errorf("affected = %d, want 4", n)
	}
	for i, tc := range tests {
		got := mustGet(t, db, ids[i])
		gotErr := ""
		if got.Error != nil {
			gotErr = *got.Error
		}
		if got.Status != tc.wantStatus || gotErr != tc.wantError {
			t.Errorf("%s: status=%s error=%q, want %s %q", tc.status, got.Status, gotErr, tc.wantStatus, tc.wantError)
		}
	}

	if n, err := db.FailUnfinishedDocuments(ctx, "dừng"); err != nil || n != 0 {
		t.Errorf("second call = %d, %v; want 0, nil", n, err)
	}
}

func TestEmbedModelAndReset(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	d, _ := db.CreateDocument(ctx, "a.pdf", 1)
	if got := mustGet(t, db, d.ID); got.EmbedModel != nil {
		t.Errorf("new document embed_model = %q, want NULL", *got.EmbedModel)
	}
	if err := db.CompleteDocument(ctx, d.ID, "gemini-embedding-001", blankPages(1), nil); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, db, d.ID); got.EmbedModel == nil || *got.EmbedModel != "gemini-embedding-001" {
		t.Errorf("embed_model = %v, want gemini-embedding-001", got.EmbedModel)
	}

	// Xử lý lại: về pending, xoá lỗi và tiến độ; trang và chunk cũ giữ tới
	// khi lần chạy mới hoàn tất.
	if err := db.FailDocument(ctx, d.ID, "lỗi cũ"); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetDocument(ctx, d.ID); err != nil {
		t.Fatalf("ResetDocument: %v", err)
	}
	got := mustGet(t, db, d.ID)
	if got.Status != StatusPending || got.Progress != 0 || got.Error != nil {
		t.Errorf("after reset = %+v", got)
	}
	if err := db.ResetDocument(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reset missing = %v, want ErrNotFound", err)
	}
}
