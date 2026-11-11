package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

// Trạng thái xử lý, khớp CHECK của cột documents.status.
const (
	StatusPending    = "pending"
	StatusExtracting = "extracting"
	StatusChunking   = "chunking"
	StatusEmbedding  = "embedding"
	StatusReady      = "ready"
	StatusFailed     = "failed"
)

type Document struct {
	ID          string     `json:"id"`
	Filename    string     `json:"filename"`
	SizeBytes   int64      `json:"size_bytes"`
	PageCount   *int       `json:"page_count"`
	ChunkCount  int        `json:"chunk_count"`
	Status      string     `json:"status"`
	Progress    int        `json:"progress"`
	Error       *string    `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ProcessedAt *time.Time `json:"processed_at"`
}

// NewChunk là một chunk chờ lưu. Embedding rỗng được lưu thành NULL.
type NewChunk struct {
	Index      int
	Content    string
	PageNumber int
	CharStart  int
	CharEnd    int
	TokenCount int
	Embedding  []float32
}

const documentColumns = `id, filename, size_bytes, page_count, chunk_count, status, progress, error, created_at, updated_at, processed_at`

func scanDocument(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.Filename, &d.SizeBytes, &d.PageCount, &d.ChunkCount, &d.Status,
		&d.Progress, &d.Error, &d.CreatedAt, &d.UpdatedAt, &d.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	return d, err
}

func (db *DB) CreateDocument(ctx context.Context, filename string, sizeBytes int64) (Document, error) {
	d, err := scanDocument(db.Pool.QueryRow(ctx,
		`INSERT INTO documents (filename, size_bytes) VALUES ($1, $2) RETURNING `+documentColumns,
		filename, sizeBytes))
	if err != nil {
		return Document{}, fmt.Errorf("create document: %w", err)
	}
	return d, nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// GetDocument trả ErrNotFound cả khi id không phải UUID hợp lệ.
func (db *DB) GetDocument(ctx context.Context, id string) (Document, error) {
	if !uuidRe.MatchString(id) {
		return Document{}, ErrNotFound
	}
	d, err := scanDocument(db.Pool.QueryRow(ctx,
		`SELECT `+documentColumns+` FROM documents WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Document{}, err
		}
		return Document{}, fmt.Errorf("get document %s: %w", id, err)
	}
	return d, nil
}

// ListDocuments trả tối đa limit tài liệu, mới nhất trước.
func (db *DB) ListDocuments(ctx context.Context, limit int) ([]Document, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+documentColumns+` FROM documents ORDER BY created_at DESC, id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	docs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Document, error) { return scanDocument(row) })
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	return docs, nil
}

// DeleteDocument xoá tài liệu; chunk bị xoá theo (ON DELETE CASCADE).
func (db *DB) DeleteDocument(ctx context.Context, id string) error {
	if !uuidRe.MatchString(id) {
		return ErrNotFound
	}
	tag, err := db.Pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDocumentStage chuyển tài liệu sang một bước xử lý. Bước đầu tiên
// (extracting) đánh dấu thời điểm bắt đầu và xoá lỗi của lần chạy trước.
func (db *DB) SetDocumentStage(ctx context.Context, id, status string, progress int) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE documents SET
			status = $2,
			progress = $3,
			processing_started_at = CASE WHEN $2 = 'extracting' THEN now() ELSE processing_started_at END,
			error = CASE WHEN $2 = 'extracting' THEN NULL ELSE error END,
			updated_at = now()
		WHERE id = $1`, id, status, progress)
	if err != nil {
		return fmt.Errorf("set document %s stage %s: %w", id, status, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("set document %s stage %s: %w", id, status, ErrNotFound)
	}
	return nil
}

func (db *DB) FailDocument(ctx context.Context, id, message string) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE documents SET status = 'failed', error = $2, processed_at = now(), updated_at = now()
		WHERE id = $1`, id, message)
	if err != nil {
		return fmt.Errorf("fail document %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("fail document %s: %w", id, ErrNotFound)
	}
	return nil
}

// CompleteDocument thay toàn bộ chunk của tài liệu và chuyển sang ready trong
// MỘT transaction: không bao giờ có trạng thái ready mà thiếu chunk, hay chunk
// dở dang của một lần chạy lỗi. Chạy lại cho cùng tài liệu là an toàn.
func (db *DB) CompleteDocument(ctx context.Context, id string, pageCount int, chunks []NewChunk) error {
	n := len(chunks)
	idx, page, start, end, tokens := make([]int32, n), make([]int32, n), make([]int32, n), make([]int32, n), make([]int32, n)
	contents := make([]string, n)
	embeddings := make([]*string, n)
	for i, c := range chunks {
		idx[i], page[i], start[i], end[i], tokens[i] = int32(c.Index), int32(c.PageNumber), int32(c.CharStart), int32(c.CharEnd), int32(c.TokenCount)
		contents[i] = c.Content
		if len(c.Embedding) > 0 {
			v := vectorLiteral(c.Embedding)
			embeddings[i] = &v
		}
	}

	err := pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		// Khoá row: tài liệu bị xoá trước đó thì báo ErrNotFound (thay vì lỗi
		// khoá ngoại), và DELETE song song phải chờ transaction này xong.
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM documents WHERE id = $1 FOR UPDATE`, id).Scan(&one); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock document: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, id); err != nil {
			return fmt.Errorf("delete old chunks: %w", err)
		}
		// Một câu INSERT ... unnest cho cả lô: một round trip dù có hàng nghìn chunk.
		if _, err := tx.Exec(ctx, `
			INSERT INTO chunks (document_id, chunk_index, content, page_number, char_start, char_end, token_count, embedding)
			SELECT $1, t.idx, t.content, t.page, t.cs, t.ce, t.tok, t.emb::vector
			FROM unnest($2::int[], $3::text[], $4::int[], $5::int[], $6::int[], $7::int[], $8::text[])
				AS t(idx, content, page, cs, ce, tok, emb)`,
			id, idx, contents, page, start, end, tokens, embeddings); err != nil {
			return fmt.Errorf("insert chunks: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE documents SET status = 'ready', progress = 100, page_count = $2, chunk_count = $3,
				error = NULL, processed_at = now(), updated_at = now()
			WHERE id = $1`, id, pageCount, n); err != nil {
			return fmt.Errorf("mark ready: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("complete document %s: %w", id, err)
	}
	return nil
}

// vectorLiteral tạo literal của pgvector, vd. "[0.1,0.2]".
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
