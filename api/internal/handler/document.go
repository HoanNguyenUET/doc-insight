package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

type DocumentService interface {
	Upload(ctx context.Context, filename string, r io.Reader) (repository.Document, error)
	List(ctx context.Context, limit int) ([]repository.Document, error)
	Get(ctx context.Context, id string) (repository.Document, error)
	Delete(ctx context.Context, id string) error
}

type DocumentHandler struct {
	svc DocumentService
}

func NewDocumentHandler(svc DocumentService) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

const (
	// Phần đầu/cuối multipart cộng thêm vào giới hạn file; service tự kiểm
	// đúng 50 MB cho phần file.
	multipartOverhead = 1 << 20
	defaultListLimit  = 50
	maxListLimit      = 200
)

// Upload nhận multipart/form-data, trường "file". Đọc dạng stream nên file
// 50 MB không bị nạp hết vào RAM.
func (h *DocumentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxUploadBytes+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, r, badRequest("cần multipart/form-data có trường file"))
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			writeError(w, r, badRequest("thiếu trường file"))
			return
		}
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				writeError(w, r, err)
			} else {
				writeError(w, r, badRequest("multipart không hợp lệ"))
			}
			return
		}
		if part.FormName() != "file" {
			continue
		}
		doc, err := h.svc.Upload(r.Context(), part.FileName(), part)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": doc.ID, "status": doc.Status})
		return
	}
}

func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := defaultListLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxListLimit {
			writeError(w, r, badRequest("limit phải là số nguyên 1-"+strconv.Itoa(maxListLimit)))
			return
		}
		limit = n
	}
	docs, err := h.svc.List(r.Context(), limit)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if docs == nil {
		docs = []repository.Document{} // JSON [] thay vì null
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	doc, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
