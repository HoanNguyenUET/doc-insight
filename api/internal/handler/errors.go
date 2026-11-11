package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

// badRequest là lỗi validate request ở tầng handler (thiếu trường, sai tham số).
type badRequest string

func (e badRequest) Error() string { return string(e) }

type errorBody struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// errorStatus là nơi DUY NHẤT map lỗi sang HTTP status.
func errorStatus(err error) int {
	var br badRequest
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &br), errors.Is(err, service.ErrInvalidFile):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrDocNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrFileTooLarge), errors.As(err, &tooBig):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, service.ErrBusy):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// writeError trả {"error": ..., "request_id": ...}. Lỗi 5xx chỉ trả câu chung
// (không lộ chi tiết nội bộ) và được ghi log kèm request_id để tra cứu.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := errorStatus(err)
	msg := err.Error()
	switch {
	case status == http.StatusRequestEntityTooLarge:
		msg = service.ErrFileTooLarge.Error()
	case status >= 500 && status != http.StatusServiceUnavailable:
		slog.ErrorContext(r.Context(), "request failed", "err", err, "method", r.Method, "path", r.URL.Path,
			"request_id", middleware.GetReqID(r.Context()))
		msg = "lỗi máy chủ nội bộ"
	}
	writeJSON(w, status, errorBody{Error: msg, RequestID: middleware.GetReqID(r.Context())})
}
