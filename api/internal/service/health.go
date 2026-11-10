package service

import (
	"context"
	"log/slog"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthReport struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

func (r HealthReport) OK() bool { return r.Status == "ok" }

type HealthService struct {
	db      Pinger
	timeout time.Duration
}

func NewHealthService(db Pinger) *HealthService {
	return &HealthService{db: db, timeout: 3 * time.Second}
}

func (s *HealthService) Check(ctx context.Context) HealthReport {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		slog.WarnContext(ctx, "health check: db unreachable", "err", err)
		return HealthReport{Status: "error", DB: "error"}
	}
	return HealthReport{Status: "ok", DB: "ok"}
}
