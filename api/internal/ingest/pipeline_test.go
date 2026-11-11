package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/HoanNguyenUET/doc-insight/api/internal/chunker"
	"github.com/HoanNguyenUET/doc-insight/api/internal/pdftest"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
)

type fakeStore struct {
	mu        sync.Mutex
	stages    []string
	completed []repository.NewChunk
	pages     []string
	model     string
	failMsg   string
	missing   bool
}

func (s *fakeStore) SetDocumentStage(_ context.Context, _ string, status string, progress int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.missing {
		return repository.ErrNotFound
	}
	s.stages = append(s.stages, fmt.Sprintf("%s:%d", status, progress))
	return nil
}

func (s *fakeStore) CompleteDocument(_ context.Context, _, model string, pages []string, chunks []repository.NewChunk) error {
	s.completed, s.pages, s.model = chunks, pages, model
	return nil
}

func (s *fakeStore) FailDocument(_ context.Context, _ string, msg string) error {
	s.failMsg = msg
	return nil
}

type recordingEmbedder struct {
	batches []int
	err     error
	short   bool
}

func (e *recordingEmbedder) EmbedModel() string { return "rec-model" }

func (e *recordingEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.batches = append(e.batches, len(texts))
	if e.err != nil {
		return nil, e.err
	}
	if e.short {
		return make([][]float32, len(texts)-1), nil
	}
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{float32(i)}
	}
	return out, nil
}

func writePDF(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestPipeline(store Store, emb Embedder, logs *bytes.Buffer) *Pipeline {
	p := NewPipeline(store, emb)
	p.log = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return p
}

// sentencesText tạo n câu, mỗi câu đánh số để nhận ra trang/vị trí.
func sentencesText(page, n int) string {
	var lines []string
	for i := 0; i < n; i++ {
		lines = append(lines, fmt.Sprintf("Trang %d cau %d noi dung mau de cat doan.", page, i))
	}
	return strings.Join(lines, "\n")
}

func TestPipelineProcess(t *testing.T) {
	pages := []string{sentencesText(1, 80), "", sentencesText(3, 131)}
	path := writePDF(t, pdftest.Build(pages...))
	store := &fakeStore{}
	emb := &recordingEmbedder{}
	var logs bytes.Buffer
	p := newTestPipeline(store, emb, &logs)
	p.chunkOpts = chunker.Options{TargetTokens: 40, OverlapTokens: 10} // nhiều chunk để thấy chia lô

	if err := p.Process(context.Background(), Job{DocumentID: "doc-1", Path: path}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Trạng thái đi đúng thứ tự, tiến độ không giảm.
	var statuses []string
	last := -1
	for _, s := range store.stages {
		var st string
		var pr int
		fmt.Sscanf(strings.Replace(s, ":", " ", 1), "%s %d", &st, &pr)
		if len(statuses) == 0 || statuses[len(statuses)-1] != st {
			statuses = append(statuses, st)
		}
		if pr < last {
			t.Errorf("progress went backwards: %v", store.stages)
		}
		last = pr
	}
	if got := strings.Join(statuses, ","); got != "extracting,chunking,embedding" {
		t.Errorf("stages = %s", got)
	}

	// Lưu đúng văn bản trang mà offset của chunk tham chiếu, kể cả trang trống.
	if store.model != "rec-model" {
		t.Errorf("embed model = %q, want the embedder's model", store.model)
	}
	if len(store.pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(store.pages))
	}
	for i, want := range pages {
		if store.pages[i] != want {
			t.Errorf("page %d text = %q, want %q", i+1, store.pages[i], want)
		}
	}
	n := len(store.completed)
	if n <= 100 || n%50 == 0 {
		t.Fatalf("only %d chunks; test needs >100, not a multiple of 50, to exercise batching", n)
	}
	// Lô 50, lô cuối phần còn lại.
	for i, b := range emb.batches {
		want := 50
		if i == len(emb.batches)-1 {
			want = n - 50*(len(emb.batches)-1)
		}
		if b != want {
			t.Errorf("batch %d size = %d, want %d (batches %v)", i, b, want, emb.batches)
		}
	}

	pageRunes := map[int][]rune{1: []rune(store.pages[0]), 3: []rune(store.pages[2])}
	for i, c := range store.completed {
		if c.Index != i {
			t.Fatalf("chunk %d has index %d", i, c.Index)
		}
		r, ok := pageRunes[c.PageNumber]
		if !ok {
			t.Fatalf("chunk %d on page %d, which has no text", i, c.PageNumber)
		}
		if string(r[c.CharStart:c.CharEnd]) != c.Content {
			t.Fatalf("chunk %d content does not match page %d [%d,%d)", i, c.PageNumber, c.CharStart, c.CharEnd)
		}
		if !strings.Contains(c.Content, fmt.Sprintf("Trang %d ", c.PageNumber)) {
			t.Fatalf("chunk %d text %q not from page %d", i, c.Content, c.PageNumber)
		}
		if len(c.Embedding) != 1 {
			t.Fatalf("chunk %d embedding not attached", i)
		}
	}

	// Mọi dòng log đều có document_id.
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) < 5 {
		t.Errorf("only %d log lines", len(lines))
	}
	for _, l := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatalf("bad log line %q", l)
		}
		if rec["document_id"] != "doc-1" {
			t.Errorf("log line without document_id: %s", l)
		}
	}
}

func TestPipelineNoopEmbedderStoresNoVectors(t *testing.T) {
	store := &fakeStore{}
	p := newTestPipeline(store, NoopEmbedder{}, &bytes.Buffer{})
	if err := p.Process(context.Background(), Job{DocumentID: "d", Path: writePDF(t, pdftest.Build("Mot cau."))}); err != nil {
		t.Fatal(err)
	}
	if len(store.completed) != 1 || store.completed[0].Embedding != nil {
		t.Errorf("completed = %+v", store.completed)
	}
}

func TestPipelineErrors(t *testing.T) {
	good := pdftest.Build("Mot cau. Hai cau.")
	tests := []struct {
		name    string
		data    []byte
		path    string
		emb     *recordingEmbedder
		missing bool
		wantErr string
		wantIs  error
	}{
		{name: "pdf without text", data: pdftest.Build("", ""), wantIs: ErrNoText},
		{name: "corrupt pdf", data: []byte("%PDF-1.4 rác"), wantErr: "trích văn bản"},
		{name: "file missing", path: "/nonexistent/x.pdf", wantErr: "mở file"},
		{name: "embedder error", data: good, emb: &recordingEmbedder{err: errors.New("quota")}, wantErr: "tạo embedding: lô 0-0: quota"},
		{name: "embedder wrong count", data: good, emb: &recordingEmbedder{short: true}, wantErr: "trả 0 vector cho 1 đoạn"},
		{name: "document deleted", data: good, missing: true, wantIs: ErrDocumentDeleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if path == "" {
				path = writePDF(t, tt.data)
			}
			emb := tt.emb
			if emb == nil {
				emb = &recordingEmbedder{}
			}
			store := &fakeStore{missing: tt.missing}
			err := newTestPipeline(store, emb, &bytes.Buffer{}).Process(context.Background(), Job{DocumentID: "d", Path: path})
			switch {
			case err == nil:
				t.Fatal("want error, got nil")
			case tt.wantIs != nil && !errors.Is(err, tt.wantIs):
				t.Errorf("err = %v, want %v", err, tt.wantIs)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			if store.completed != nil {
				t.Error("chunks saved despite error")
			}
		})
	}
}

func TestPipelineFailTruncatesMessage(t *testing.T) {
	store := &fakeStore{}
	p := NewPipeline(store, NoopEmbedder{})
	if err := p.Fail(context.Background(), "d", strings.Repeat("ồ", 5000)); err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(store.failMsg); n != maxErrorMessage+1 || !utf8.ValidString(store.failMsg) {
		t.Errorf("stored message has %d runes (valid UTF-8: %v)", n, utf8.ValidString(store.failMsg))
	}
}
