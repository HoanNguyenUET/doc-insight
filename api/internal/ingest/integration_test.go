package ingest_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/internal/extract"
	"github.com/HoanNguyenUET/doc-insight/api/internal/ingest"
	"github.com/HoanNguyenUET/doc-insight/api/internal/pdftest"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
	"github.com/HoanNguyenUET/doc-insight/api/internal/testdb"
	"github.com/HoanNguyenUET/doc-insight/api/migrations"
)

// Toàn bộ luồng như production: service lưu file + tạo row + enqueue, pool 4
// worker chạy pipeline thật, ghi vào Postgres thật.
func setup(t *testing.T) (*service.DocumentService, *repository.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := repository.NewDB(ctx, testdb.URL(t), 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx, migrations.FS); err != nil {
		t.Fatal(err)
	}

	pool := ingest.NewPool(ingest.NewPipeline(db, ingest.NoopEmbedder{}), 4, 100, time.Minute)
	poolCtx, cancel := context.WithCancel(ctx)
	pool.Start(poolCtx)
	t.Cleanup(func() { cancel(); pool.Wait() })

	return service.NewDocumentService(db, pool, t.TempDir()), db
}

func waitFinal(t *testing.T, db *repository.DB, id string) repository.Document {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		d, err := db.GetDocument(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if d.Status == repository.StatusReady || d.Status == repository.StatusFailed {
			return d
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("document %s not finished in time", id)
	return repository.Document{}
}

func bookPage(page, sentences int) string {
	var lines []string
	for i := 0; i < sentences; i++ {
		lines = append(lines, fmt.Sprintf("Page %d sentence %d talks about retrieval and citations.", page, i))
		if i%10 == 9 {
			lines = append(lines, "") // ngắt đoạn
		}
	}
	return strings.Join(lines, "\n")
}

func TestIngestEndToEnd(t *testing.T) {
	t.Parallel()
	svc, db := setup(t)
	ctx := context.Background()

	pages := []string{bookPage(1, 70), "", bookPage(3, 5), bookPage(4, 120)}
	data := pdftest.Build(pages...)
	doc, err := svc.Upload(ctx, "book.pdf", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if doc.Status != repository.StatusPending {
		t.Errorf("status right after upload = %s, want pending", doc.Status)
	}

	got := waitFinal(t, db, doc.ID)
	if got.Status != repository.StatusReady || got.Error != nil || got.Progress != 100 {
		t.Fatalf("final = %+v (error %v)", got, got.Error)
	}
	if got.PageCount == nil || *got.PageCount != 4 {
		t.Errorf("page_count = %v, want 4", got.PageCount)
	}

	// Đối chiếu từng chunk với văn bản trích lại từ chính file đó.
	want, err := extract.PDF(ctx, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	pageText := map[int][]rune{}
	for _, p := range want {
		pageText[p.Number] = []rune(p.Text)
	}

	rows, err := db.Pool.Query(ctx, `SELECT chunk_index, page_number, char_start, char_end, content, token_count, embedding IS NULL
		FROM chunks WHERE document_id = $1 ORDER BY chunk_index`, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	pagesSeen := map[int]bool{}
	for rows.Next() {
		var idx, page, cs, ce, tok int
		var content string
		var embNull bool
		if err := rows.Scan(&idx, &page, &cs, &ce, &content, &tok, &embNull); err != nil {
			t.Fatal(err)
		}
		if idx != n {
			t.Fatalf("chunk_index %d at position %d", idx, n)
		}
		if string(pageText[page][cs:ce]) != content {
			t.Fatalf("chunk %d: content != page %d text[%d:%d]", idx, page, cs, ce)
		}
		if !strings.Contains(content, fmt.Sprintf("Page %d ", page)) {
			t.Fatalf("chunk %d stored as page %d but text is %q", idx, page, content[:30])
		}
		if tok > 500 || !embNull {
			t.Fatalf("chunk %d: tokens %d, embedding null %v", idx, tok, embNull)
		}
		pagesSeen[page] = true
		n++
	}
	if n != got.ChunkCount || n < 4 {
		t.Errorf("%d chunk rows, chunk_count = %d", n, got.ChunkCount)
	}
	if !pagesSeen[1] || pagesSeen[2] || !pagesSeen[3] || !pagesSeen[4] {
		t.Errorf("pages with chunks = %v, want 1,3,4", pagesSeen)
	}
}

func TestIngestFailuresAreRecorded(t *testing.T) {
	t.Parallel()
	svc, db := setup(t)
	ctx := context.Background()

	tests := map[string]struct {
		data    []byte
		wantErr string
	}{
		"scanned (no text)":   {pdftest.Build("", ""), "không có lớp văn bản"},
		"corrupt after magic": {[]byte("%PDF-1.4\nđây không phải PDF thật"), "trích văn bản"},
	}
	for name, tt := range tests {
		doc, err := svc.Upload(ctx, name+".pdf", bytes.NewReader(tt.data))
		if err != nil {
			t.Fatalf("%s: Upload: %v", name, err)
		}
		got := waitFinal(t, db, doc.ID)
		if got.Status != repository.StatusFailed || got.Error == nil || !strings.Contains(*got.Error, tt.wantErr) {
			t.Errorf("%s: final = %s, error %v; want failed with %q", name, got.Status, got.Error, tt.wantErr)
		}
		if got.ChunkCount != 0 {
			t.Errorf("%s: chunk_count = %d", name, got.ChunkCount)
		}
	}
}

// Nhiều tài liệu cùng lúc qua pool 4 worker: tất cả đều xong, không lẫn chunk.
func TestIngestManyDocuments(t *testing.T) {
	t.Parallel()
	svc, db := setup(t)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 12; i++ {
		doc, err := svc.Upload(ctx, fmt.Sprintf("d%d.pdf", i), bytes.NewReader(pdftest.Build(bookPage(1, 20+i*5))))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, doc.ID)
	}
	for _, id := range ids {
		if d := waitFinal(t, db, id); d.Status != repository.StatusReady {
			t.Errorf("%s: %s %v", id, d.Status, d.Error)
		}
	}
	var total, sum int
	_ = db.Pool.QueryRow(ctx, `SELECT count(*) FROM chunks`).Scan(&total)
	_ = db.Pool.QueryRow(ctx, `SELECT sum(chunk_count) FROM documents`).Scan(&sum)
	if total != sum {
		t.Errorf("chunks table has %d rows, documents.chunk_count sums to %d", total, sum)
	}
}
