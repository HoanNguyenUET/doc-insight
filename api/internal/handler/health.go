package handler

import (
	"context"
	"net/http"

	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

type HealthChecker interface {
	Check(ctx context.Context) service.HealthReport
}

type HealthHandler struct {
	svc HealthChecker
}

func NewHealthHandler(svc HealthChecker) *HealthHandler {
	return &HealthHandler{svc: svc}
}

// Healthz trả 200 {"status":"ok","db":"ok"}, hoặc 503 khi không ping được DB.
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	report := h.svc.Check(r.Context())
	status := http.StatusOK
	if !report.OK() {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}
