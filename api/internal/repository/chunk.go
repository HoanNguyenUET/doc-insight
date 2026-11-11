package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ChunkMatch là một chunk tìm được theo độ tương đồng với vector truy vấn.
type ChunkMatch struct {
	ID         int64
	Index      int
	Content    string
	PageNumber int
	CharStart  int
	CharEnd    int
	// Score là độ tương đồng cosine (1 - khoảng cách), càng lớn càng gần.
	Score float64
}

// SearchChunks trả tối đa limit chunk của tài liệu gần embedding nhất.
// Chunk chưa có embedding bị bỏ qua.
//
// CTE MATERIALIZED buộc lọc theo tài liệu TRƯỚC rồi mới so khoảng cách
// (quét chính xác trên các chunk của một tài liệu). Nếu để planner dùng index
// ivfflat, nó chỉ dò vài cụm trên toàn bảng rồi mới lọc document_id, nên có
// thể trả ít hơn limit dòng, thậm chí không dòng nào.
func (db *DB) SearchChunks(ctx context.Context, documentID string, embedding []float32, limit int) ([]ChunkMatch, error) {
	if !uuidRe.MatchString(documentID) {
		return nil, ErrNotFound
	}
	rows, err := db.Pool.Query(ctx, `
		WITH doc_chunks AS MATERIALIZED (
			SELECT id, chunk_index, content, page_number, char_start, char_end, embedding
			FROM chunks
			WHERE document_id = $1 AND embedding IS NOT NULL
		)
		SELECT id, chunk_index, content, page_number, char_start, char_end, embedding <=> $2::text::vector AS distance
		FROM doc_chunks
		ORDER BY distance, chunk_index
		LIMIT $3`, documentID, vectorLiteral(embedding), limit)
	if err != nil {
		return nil, fmt.Errorf("search chunks %s: %w", documentID, err)
	}
	matches, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ChunkMatch, error) {
		var m ChunkMatch
		var distance float64
		err := row.Scan(&m.ID, &m.Index, &m.Content, &m.PageNumber, &m.CharStart, &m.CharEnd, &distance)
		m.Score = 1 - distance
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("search chunks %s: %w", documentID, err)
	}
	return matches, nil
}
