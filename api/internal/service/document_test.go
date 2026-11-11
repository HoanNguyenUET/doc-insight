package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/HoanNguyenUET/doc-insight/api/internal/ingest"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
)

type fakeRepo struct {
	mu        sync.Mutex
	docs      map[string]repository.Document
	createErr error
	nextID    int
	pages     map[string][]string
	chunks    []repository.Chunk
}

func newFakeRepo() *fakeRepo { return &fakeRepo{docs: map[string]repository.Document{}} }

func (r *fakeRepo) CreateDocument(_ context.Context, filename string, size int64) (repository.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return repository.Document{}, r.createErr
	}
	r.nextID++
	d := repository.Document{ID: "00000000-0000-0000-0000-00000000000" + string(rune('0'+r.nextID)), Filename: filename, SizeBytes: size, Status: repository.StatusPending}
	r.docs[d.ID] = d
	return d, nil
}

func (r *fakeRepo) GetDocument(_ context.Context, id string) (repository.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.docs[id]
	if !ok {
		return repository.Document{}, repository.ErrNotFound
	}
	return d, nil
}

func (r *fakeRepo) ListDocuments(context.Context, int) ([]repository.Document, error) {
	return nil, nil
}

func (r *fakeRepo) DeleteDocument(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.docs[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.docs, id)
	return nil
}

func (r *fakeRepo) GetPage(_ context.Context, id string, number int) (repository.Page, error) {
	pages := r.pages[id]
	if number < 1 || number > len(pages) {
		return repository.Page{}, repository.ErrNotFound
	}
	return repository.Page{Number: number, Text: pages[number-1]}, nil
}

func (r *fakeRepo) ListChunks(_ context.Context, _ string, page int) ([]repository.Chunk, error) {
	var out []repository.Chunk
	for _, c := range r.chunks {
		if page == 0 || c.PageNumber == page {
			out = append(out, c)
		}
	}
	return out, nil
}

type fakeQueue struct {
	jobs []ingest.Job
	err  error
}

func (q *fakeQueue) Enqueue(job ingest.Job) error {
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, job)
	return nil
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func newSvc(t *testing.T) (*DocumentService, *fakeRepo, *fakeQueue, string) {
	dir := t.TempDir()
	repo, queue := newFakeRepo(), &fakeQueue{}
	return NewDocumentService(repo, queue, dir), repo, queue, dir
}

var samplePDF = []byte("%PDF-1.4\nnội dung giả\n%%EOF")

func TestUploadStoresFileAndEnqueues(t *testing.T) {
	svc, repo, queue, dir := newSvc(t)

	doc, err := svc.Upload(context.Background(), `C:\Users\x\Báo cáo.pdf`, bytes.NewReader(samplePDF))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if doc.Status != repository.StatusPending || doc.Filename != "Báo cáo.pdf" || doc.SizeBytes != int64(len(samplePDF)) {
		t.Errorf("doc = %+v", doc)
	}
	if _, ok := repo.docs[doc.ID]; !ok {
		t.Error("row not created")
	}
	got, err := os.ReadFile(svc.FilePath(doc.ID))
	if err != nil || !bytes.Equal(got, samplePDF) {
		t.Errorf("stored file = %q, %v", got, err)
	}
	if len(queue.jobs) != 1 || queue.jobs[0] != (ingest.Job{DocumentID: doc.ID, Path: svc.FilePath(doc.ID)}) {
		t.Errorf("jobs = %+v", queue.jobs)
	}
	if names := dirEntries(t, dir); len(names) != 1 || names[0] != doc.ID+".pdf" {
		t.Errorf("storage dir = %v, want only %s.pdf (no temp files)", names, doc.ID)
	}
}

func TestUploadRejectsWithoutLeftovers(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		max     int64
		setup   func(*fakeRepo, *fakeQueue)
		wantIs  error
		wantMsg string
	}{
		{name: "not a pdf", body: []byte("PK\x03\x04 zip file"), wantIs: ErrInvalidFile, wantMsg: "không phải PDF"},
		{name: "html renamed .pdf", body: []byte("<html>%PDF-</html>"), wantIs: ErrInvalidFile},
		{name: "empty", body: nil, wantIs: ErrInvalidFile, wantMsg: "rỗng"},
		{name: "shorter than magic", body: []byte("%PD"), wantIs: ErrInvalidFile},
		{name: "one byte over limit", body: append([]byte("%PDF-"), make([]byte, 96)...), max: 100, wantIs: ErrFileTooLarge},
		{name: "queue full", body: samplePDF, setup: func(_ *fakeRepo, q *fakeQueue) { q.err = ingest.ErrQueueFull }, wantIs: ErrBusy},
		{name: "db error", body: samplePDF, setup: func(r *fakeRepo, _ *fakeQueue) { r.createErr = errors.New("db down") }, wantMsg: "db down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, queue, dir := newSvc(t)
			if tt.max > 0 {
				svc.maxBytes = tt.max
			}
			if tt.setup != nil {
				tt.setup(repo, queue)
			}
			_, err := svc.Upload(context.Background(), "a.pdf", bytes.NewReader(tt.body))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, want %v", err, tt.wantIs)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("err = %q, want it to contain %q", err, tt.wantMsg)
			}
			if len(repo.docs) != 0 {
				t.Errorf("rows left: %v", repo.docs)
			}
			if names := dirEntries(t, dir); len(names) != 0 {
				t.Errorf("files left in storage: %v", names)
			}
		})
	}
}

func TestUploadExactlyAtLimit(t *testing.T) {
	svc, _, _, _ := newSvc(t)
	svc.maxBytes = 100
	if _, err := svc.Upload(context.Background(), "a.pdf", bytes.NewReader(append([]byte("%PDF-"), make([]byte, 95)...))); err != nil {
		t.Fatalf("100-byte file with 100-byte limit: %v", err)
	}
}

func TestGetAndDelete(t *testing.T) {
	svc, repo, _, dir := newSvc(t)
	ctx := context.Background()
	doc, err := svc.Upload(ctx, "a.pdf", bytes.NewReader(samplePDF))
	if err != nil {
		t.Fatal(err)
	}

	if got, err := svc.Get(ctx, doc.ID); err != nil || got.ID != doc.ID {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if err := svc.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(repo.docs) != 0 || len(dirEntries(t, dir)) != 0 {
		t.Error("row or file left after delete")
	}
	if _, err := svc.Get(ctx, doc.ID); !errors.Is(err, ErrDocNotFound) {
		t.Errorf("Get deleted err = %v, want ErrDocNotFound", err)
	}
	if err := svc.Delete(ctx, doc.ID); !errors.Is(err, ErrDocNotFound) {
		t.Errorf("Delete twice err = %v, want ErrDocNotFound", err)
	}

	// Row còn nhưng file đã mất (vd. đĩa tạm của Render bị xoá): vẫn xoá được.
	doc2, _ := svc.Upload(ctx, "b.pdf", bytes.NewReader(samplePDF))
	os.Remove(svc.FilePath(doc2.ID))
	if err := svc.Delete(ctx, doc2.ID); err != nil {
		t.Errorf("Delete with missing file: %v", err)
	}
}

func TestCleanFilename(t *testing.T) {
	tests := map[string]string{
		"report.pdf":             "report.pdf",
		"  Báo cáo quý 3.pdf  ":  "Báo cáo quý 3.pdf",
		"../../etc/passwd":       "passwd",
		`C:\Users\a\doc.pdf`:     "doc.pdf",
		"a\x00b\nc.pdf":          "abc.pdf",
		"":                       "document.pdf",
		"/":                      "document.pdf",
		strings.Repeat("ả", 300): strings.Repeat("ả", 255),
		"bad\xffutf8.pdf":        "badutf8.pdf",
	}
	for in, want := range tests {
		if got := cleanFilename(in); got != want {
			t.Errorf("cleanFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPage(t *testing.T) {
	three := 3
	repo := newFakeRepo()
	repo.docs["ready"] = repository.Document{ID: "ready", Status: repository.StatusReady, PageCount: &three}
	repo.docs["legacy"] = repository.Document{ID: "legacy", Status: repository.StatusReady, PageCount: &three}
	repo.docs["busy"] = repository.Document{ID: "busy", Status: repository.StatusEmbedding}
	repo.pages = map[string][]string{"ready": {"một", "", "ba"}}
	repo.chunks = []repository.Chunk{{Index: 0, PageNumber: 1}, {Index: 1, PageNumber: 3}, {Index: 2, PageNumber: 3}}
	svc := NewDocumentService(repo, &fakeQueue{}, t.TempDir())

	tests := []struct {
		name       string
		id         string
		number     int
		wantErr    error
		wantText   string
		wantChunks int
	}{
		{"page with chunks", "ready", 3, nil, "ba", 2},
		{"empty page", "ready", 2, nil, "", 0},
		{"zero", "ready", 0, ErrPageNotFound, "", 0},
		{"past end", "ready", 4, ErrPageNotFound, "", 0},
		{"processed before pages table", "legacy", 1, ErrPageNotFound, "", 0},
		{"not ready", "busy", 1, ErrDocNotReady, "", 0},
		{"missing", "nope", 1, ErrDocNotFound, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := svc.Page(context.Background(), tt.id, tt.number)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if p.Text != tt.wantText || len(p.Chunks) != tt.wantChunks || p.Chunks == nil || p.PageCount != 3 || p.Number != tt.number {
				t.Errorf("page = %+v", p)
			}
		})
	}
}
