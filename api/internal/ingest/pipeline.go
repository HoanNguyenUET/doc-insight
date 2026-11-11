package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
	"unicode/utf8"

	"github.com/HoanNguyenUET/doc-insight/api/internal/chunker"
	"github.com/HoanNguyenUET/doc-insight/api/internal/extract"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
)

var ErrNoText = errors.New("PDF không có lớp văn bản (có thể là bản scan); chưa hỗ trợ OCR")

// Store là phần repository mà pipeline cần.
type Store interface {
	SetDocumentStage(ctx context.Context, id, status string, progress int) error
	CompleteDocument(ctx context.Context, id string, pageCount int, chunks []repository.NewChunk) error
	FailDocument(ctx context.Context, id, message string) error
}

// Embedder tạo embedding cho một lô văn bản, trả đúng một vector cho mỗi
// văn bản theo cùng thứ tự. Vector rỗng nghĩa là chưa có embedding (lưu NULL).
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// NoopEmbedder trả vector rỗng cho mọi văn bản (chunk lưu embedding NULL);
// dùng trong test không cần embedding.
type NoopEmbedder struct{}

func (NoopEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

const (
	embedBatchSize  = 50
	maxErrorMessage = 2000 // rune
)

// Tiến độ (%) khi vào từng bước; bước embedding tăng dần theo lô tới 95.
const (
	progressExtracting = 5
	progressChunking   = 40
	progressEmbedding  = 50
	progressEmbedDone  = 95
)

type Pipeline struct {
	store     Store
	embedder  Embedder
	chunkOpts chunker.Options
	batchSize int
	log       *slog.Logger
}

func NewPipeline(store Store, embedder Embedder) *Pipeline {
	return &Pipeline{
		store:     store,
		embedder:  embedder,
		chunkOpts: chunker.DefaultOptions,
		batchSize: embedBatchSize,
		log:       slog.Default(),
	}
}

func (p *Pipeline) Fail(ctx context.Context, documentID, message string) error {
	if utf8.RuneCountInString(message) > maxErrorMessage {
		message = string([]rune(message)[:maxErrorMessage]) + "…"
	}
	return p.store.FailDocument(ctx, documentID, message)
}

func (p *Pipeline) Process(ctx context.Context, job Job) error {
	log := p.log.With("document_id", job.DocumentID)

	if err := p.stage(ctx, log, job.DocumentID, repository.StatusExtracting, progressExtracting); err != nil {
		return err
	}
	t := time.Now()
	pages, err := p.extract(ctx, job.Path)
	if err != nil {
		return fmt.Errorf("trích văn bản: %w", err)
	}
	log.InfoContext(ctx, "ingest: extracted", "pages", len(pages), "duration_ms", time.Since(t).Milliseconds())

	if err := p.stage(ctx, log, job.DocumentID, repository.StatusChunking, progressChunking); err != nil {
		return err
	}
	var chunks []chunker.Chunk
	for _, pg := range pages {
		chunks = append(chunks, chunker.SplitPage(pg.Number, pg.Text, p.chunkOpts)...)
	}
	if len(chunks) == 0 {
		return ErrNoText
	}
	log.InfoContext(ctx, "ingest: chunked", "chunks", len(chunks))

	if err := p.stage(ctx, log, job.DocumentID, repository.StatusEmbedding, progressEmbedding); err != nil {
		return err
	}
	t = time.Now()
	rows, err := p.embed(ctx, log, job.DocumentID, chunks)
	if err != nil {
		return fmt.Errorf("tạo embedding: %w", err)
	}
	log.InfoContext(ctx, "ingest: embedded", "chunks", len(rows), "duration_ms", time.Since(t).Milliseconds())

	if err := p.store.CompleteDocument(ctx, job.DocumentID, len(pages), rows); err != nil {
		return deletedOr(err, "lưu chunk")
	}
	log.InfoContext(ctx, "ingest: saved", "chunks", len(rows), "status", repository.StatusReady)
	return nil
}

func (p *Pipeline) stage(ctx context.Context, log *slog.Logger, id, status string, progress int) error {
	log.InfoContext(ctx, "ingest: stage", "status", status)
	if err := p.store.SetDocumentStage(ctx, id, status, progress); err != nil {
		return deletedOr(err, "cập nhật trạng thái "+status)
	}
	return nil
}

func (p *Pipeline) extract(ctx context.Context, path string) ([]extract.Page, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("mở file: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	return extract.PDF(ctx, f, st.Size())
}

// embed gọi embedder theo lô batchSize chunk và cập nhật tiến độ sau mỗi lô.
func (p *Pipeline) embed(ctx context.Context, log *slog.Logger, id string, chunks []chunker.Chunk) ([]repository.NewChunk, error) {
	rows := make([]repository.NewChunk, len(chunks))
	for start := 0; start < len(chunks); start += p.batchSize {
		end := min(start+p.batchSize, len(chunks))
		texts := make([]string, 0, end-start)
		for _, c := range chunks[start:end] {
			texts = append(texts, c.Content)
		}
		vecs, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("lô %d-%d: %w", start, end-1, err)
		}
		if len(vecs) != len(texts) {
			return nil, fmt.Errorf("lô %d-%d: embedder trả %d vector cho %d đoạn", start, end-1, len(vecs), len(texts))
		}
		for i, c := range chunks[start:end] {
			rows[start+i] = repository.NewChunk{
				Index:      start + i,
				Content:    c.Content,
				PageNumber: c.Page,
				CharStart:  c.Start,
				CharEnd:    c.End,
				TokenCount: c.Tokens,
				Embedding:  vecs[i],
			}
		}
		log.DebugContext(ctx, "ingest: embedded batch", "from", start, "to", end-1)

		progress := progressEmbedding + (progressEmbedDone-progressEmbedding)*end/len(chunks)
		if err := p.store.SetDocumentStage(ctx, id, repository.StatusEmbedding, progress); err != nil {
			return nil, deletedOr(err, "cập nhật tiến độ")
		}
	}
	return rows, nil
}

func deletedOr(err error, what string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrDocumentDeleted
	}
	return fmt.Errorf("%s: %w", what, err)
}
