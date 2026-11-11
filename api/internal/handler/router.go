// Package handler chứa HTTP router và các handler. Handler chỉ validate
// request và gọi service; logic nằm ở service, SQL nằm ở repository.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Health         *HealthHandler
	Documents      *DocumentHandler
	Ask            *AskHandler
	AllowedOrigins []string
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestIDHeader)
	r.Use(middleware.RealIP)
	r.Use(requestLogger)
	r.Use(recoverer)
	r.Use(cors(d.AllowedOrigins))

	r.Get("/healthz", d.Health.Healthz)
	if d.Documents != nil {
		r.Route("/api/documents", func(r chi.Router) {
			r.Post("/", d.Documents.Upload)
			r.Get("/", d.Documents.List)
			r.Get("/{id}", d.Documents.Get)
			r.Delete("/{id}", d.Documents.Delete)
			r.Get("/{id}/pages/{page}", d.Documents.Page)
			r.Get("/{id}/chunks", d.Documents.Chunks)
			if d.Ask != nil {
				r.Get("/{id}/ask", d.Ask.Ask)
			}
		})
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "không tìm thấy", RequestID: middleware.GetReqID(r.Context())})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "phương thức không được hỗ trợ", RequestID: middleware.GetReqID(r.Context())})
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}
