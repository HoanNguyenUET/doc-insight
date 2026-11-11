package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Page là văn bản đã chuẩn hoá của một trang. Offset của chunk trên trang
// này tính theo rune trên Text.
type Page struct {
	Number int
	Text   string
}

// Chunk là một chunk đã lưu, không kèm embedding.
type Chunk struct {
	ID         int64
	Index      int
	PageNumber int
	CharStart  int
	CharEnd    int
	TokenCount *int
	Content    string
}

// GetPage trả ErrNotFound khi tài liệu không có trang number (kể cả tài liệu
// xử lý trước khi có bảng pages).
func (db *DB) GetPage(ctx context.Context, documentID string, number int) (Page, error) {
	if !uuidRe.MatchString(documentID) {
		return Page{}, ErrNotFound
	}
	p := Page{Number: number}
	err := db.Pool.QueryRow(ctx,
		`SELECT text FROM pages WHERE document_id = $1 AND page_number = $2`, documentID, number).Scan(&p.Text)
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("get page %s/%d: %w", documentID, number, err)
	}
	return p, nil
}

// ListChunks trả các chunk của tài liệu theo thứ tự chunk_index; page > 0 thì
// chỉ lấy chunk của trang đó.
func (db *DB) ListChunks(ctx context.Context, documentID string, page int) ([]Chunk, error) {
	if !uuidRe.MatchString(documentID) {
		return nil, ErrNotFound
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, chunk_index, page_number, char_start, char_end, token_count, content
		FROM chunks
		WHERE document_id = $1 AND ($2 = 0 OR page_number = $2)
		ORDER BY chunk_index`, documentID, page)
	if err != nil {
		return nil, fmt.Errorf("list chunks %s: %w", documentID, err)
	}
	chunks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Chunk, error) {
		var c Chunk
		err := row.Scan(&c.ID, &c.Index, &c.PageNumber, &c.CharStart, &c.CharEnd, &c.TokenCount, &c.Content)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("list chunks %s: %w", documentID, err)
	}
	return chunks, nil
}
