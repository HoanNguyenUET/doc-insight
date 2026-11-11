package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

type AskService interface {
	Ask(ctx context.Context, documentID, question string, w service.AnswerWriter) error
}

type AskHandler struct {
	svc AskService
}

func NewAskHandler(svc AskService) *AskHandler {
	return &AskHandler{svc: svc}
}

const previewRunes = 200

// Ask stream câu trả lời qua SSE: sources → token... → done, hoặc error.
//
// Lỗi xảy ra trước event đầu tiên (câu hỏi rỗng, tài liệu không có hoặc chưa
// xử lý xong, embed lỗi) trả JSON với HTTP status như các endpoint khác;
// EventSource gặp status khác 200 thì dừng hẳn, không tự kết nối lại. Lỗi
// sau khi đã stream thì gửi event error. Client ngắt giữa chừng thì không
// gửi gì thêm.
func (h *AskHandler) Ask(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, errors.New("response writer không hỗ trợ flush"))
		return
	}
	sw := &sseWriter{ctx: r.Context(), w: w, flusher: flusher, start: time.Now()}

	err := h.svc.Ask(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("q"), sw)
	switch {
	case err == nil:
		if err := sw.event("done", doneEvent{Tokens: sw.tokens, MS: time.Since(sw.start).Milliseconds()}); err != nil {
			slog.DebugContext(r.Context(), "ask: send done", "err", err)
		}
	case r.Context().Err() != nil:
		slog.InfoContext(r.Context(), "ask: client disconnected", "tokens", sw.tokens,
			"request_id", middleware.GetReqID(r.Context()))
	case !sw.started && errorStatus(err) < http.StatusInternalServerError:
		// Lỗi do request (câu hỏi sai, tài liệu không có/chưa xong): JSON kèm
		// đúng HTTP status.
		writeError(w, r, err)
	default:
		// Lỗi phía máy chủ, kể cả trước event đầu tiên: gửi event error vì
		// EventSource không đọc được body của response khác 200.
		_, msg := publicError(r, err)
		if err := sw.event("error", errorEvent{Message: msg, RequestID: middleware.GetReqID(r.Context())}); err != nil {
			slog.DebugContext(r.Context(), "ask: send error event", "err", err)
		}
	}
}

type sourceEvent struct {
	Ref       int     `json:"ref"`
	Chunk     int     `json:"chunk"`
	Page      int     `json:"page"`
	CharStart int     `json:"char_start"`
	CharEnd   int     `json:"char_end"`
	Preview   string  `json:"preview"`
	Score     float64 `json:"score"`
}

type tokenEvent struct {
	Text string `json:"text"`
}

type doneEvent struct {
	Tokens int   `json:"tokens"`
	MS     int64 `json:"ms"`
}

type errorEvent struct {
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// sseWriter cài đặt service.AnswerWriter. Header SSE chỉ được ghi ở event
// đầu tiên, để lỗi trước đó vẫn trả được JSON với đúng HTTP status.
type sseWriter struct {
	ctx     context.Context
	w       http.ResponseWriter
	flusher http.Flusher
	start   time.Time
	started bool
	tokens  int
}

func (s *sseWriter) WriteSources(sources []service.Source) error {
	chunks := make([]sourceEvent, len(sources))
	for i, src := range sources {
		chunks[i] = sourceEvent{
			Ref:       src.N,
			Chunk:     src.ChunkIndex,
			Page:      src.Page,
			CharStart: src.CharStart,
			CharEnd:   src.CharEnd,
			Preview:   preview(src.Content),
			Score:     math.Round(src.Score*100) / 100,
		}
	}
	return s.event("sources", map[string]any{"chunks": chunks})
}

func (s *sseWriter) WriteToken(text string) error {
	if err := s.event("token", tokenEvent{Text: text}); err != nil {
		return err
	}
	s.tokens++
	return nil
}

// event ghi một event rồi flush ngay. Trả lỗi khi client đã ngắt để service
// huỷ việc sinh câu trả lời.
func (s *sseWriter) event(name string, v any) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode event %s: %w", name, err)
	}
	if !s.started {
		h := s.w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no") // thiếu thì nginx đệm, stream không chạy
		s.w.WriteHeader(http.StatusOK)
		s.started = true
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return fmt.Errorf("write event %s: %w", name, err)
	}
	s.flusher.Flush()
	return nil
}

// preview rút gọn nội dung chunk còn tối đa previewRunes ký tự, gộp khoảng
// trắng; cắt theo rune để không vỡ chữ tiếng Việt.
func preview(content string) string {
	s := strings.Join(strings.Fields(content), " ")
	if utf8.RuneCountInString(s) <= previewRunes {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:previewRunes])) + "…"
}
