package handler

import (
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

// Hợp đồng JSON của API nằm ở đây, tách khỏi struct của repository và
// service: đổi tên cột hay trường nội bộ không làm đổi API.

type documentResponse struct {
	ID                  string     `json:"id"`
	Filename            string     `json:"filename"`
	SizeBytes           int64      `json:"size_bytes"`
	PageCount           *int       `json:"page_count"`
	ChunkCount          int        `json:"chunk_count"`
	Status              string     `json:"status"`
	Progress            int        `json:"progress"`
	Error               *string    `json:"error"`
	EmbedModel          *string    `json:"embed_model"`
	NeedsReprocess      bool       `json:"needs_reprocess"`
	ProcessingStartedAt *time.Time `json:"processing_started_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ProcessedAt         *time.Time `json:"processed_at"`
}

func newDocumentResponse(v service.DocumentView) documentResponse {
	d := v.Document
	return documentResponse{
		ID: d.ID, Filename: d.Filename, SizeBytes: d.SizeBytes, PageCount: d.PageCount, ChunkCount: d.ChunkCount,
		Status: d.Status, Progress: d.Progress, Error: d.Error, EmbedModel: d.EmbedModel, NeedsReprocess: v.NeedsReprocess,
		ProcessingStartedAt: d.ProcessingStartedAt, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, ProcessedAt: d.ProcessedAt,
	}
}

type chunkResponse struct {
	ID         int64  `json:"id"`
	Index      int    `json:"index"`
	PageNumber int    `json:"page_number"`
	CharStart  int    `json:"char_start"`
	CharEnd    int    `json:"char_end"`
	TokenCount *int   `json:"token_count"`
	Content    string `json:"content"`
}

// newChunkResponses luôn trả slice khác nil để JSON là [] thay vì null.
func newChunkResponses(chunks []repository.Chunk) []chunkResponse {
	out := make([]chunkResponse, len(chunks))
	for i, c := range chunks {
		out[i] = chunkResponse{ID: c.ID, Index: c.Index, PageNumber: c.PageNumber, CharStart: c.CharStart,
			CharEnd: c.CharEnd, TokenCount: c.TokenCount, Content: c.Content}
	}
	return out
}

type pageResponse struct {
	DocumentID string          `json:"document_id"`
	PageCount  int             `json:"page_count"`
	Number     int             `json:"page_number"`
	Text       string          `json:"text"`
	Chunks     []chunkResponse `json:"chunks"`
}

func newPageResponse(p service.PageView) pageResponse {
	return pageResponse{DocumentID: p.DocumentID, PageCount: p.PageCount, Number: p.Number, Text: p.Text,
		Chunks: newChunkResponses(p.Chunks)}
}
