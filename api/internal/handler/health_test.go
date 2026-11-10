package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

type pingFunc func(ctx context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func newTestRouter(pingErr error) http.Handler {
	svc := service.NewHealthService(pingFunc(func(context.Context) error { return pingErr }))
	return NewRouter(Deps{Health: NewHealthHandler(svc)})
}

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   map[string]string
	}{
		{"db ok", nil, http.StatusOK, map[string]string{"status": "ok", "db": "ok"}},
		{"db down", context.DeadlineExceeded, http.StatusServiceUnavailable, map[string]string{"status": "error", "db": "error"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestRouter(tt.pingErr).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", ct)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body %q: %v", rec.Body.String(), err)
			}
			if len(body) != len(tt.wantBody) || body["status"] != tt.wantBody["status"] || body["db"] != tt.wantBody["db"] {
				t.Errorf("body = %v, want %v", body, tt.wantBody)
			}
		})
	}
}

func TestHealthzOnlyGET(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz status = %d, want 405", rec.Code)
	}
}

func TestUnknownRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /nope status = %d, want 404", rec.Code)
	}
}
