package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type pingFunc func(ctx context.Context) error

func (f pingFunc) Ping(ctx context.Context) error { return f(ctx) }

func TestHealthCheckOK(t *testing.T) {
	svc := NewHealthService(pingFunc(func(context.Context) error { return nil }))

	got := svc.Check(context.Background())
	if got != (HealthReport{Status: "ok", DB: "ok"}) || !got.OK() {
		t.Errorf("Check = %+v, want ok/ok", got)
	}
}

func TestHealthCheckDBError(t *testing.T) {
	svc := NewHealthService(pingFunc(func(context.Context) error { return errors.New("connection refused") }))

	got := svc.Check(context.Background())
	if got != (HealthReport{Status: "error", DB: "error"}) || got.OK() {
		t.Errorf("Check = %+v, want error/error", got)
	}
}

// Một DB treo (vd. Neon đang khởi động compute) không được làm healthz treo theo.
func TestHealthCheckTimesOutHangingDB(t *testing.T) {
	svc := NewHealthService(pingFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}))
	svc.timeout = 50 * time.Millisecond

	start := time.Now()
	got := svc.Check(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Check took %v, want it bounded by the timeout", elapsed)
	}
	if got.OK() {
		t.Errorf("Check = %+v, want error", got)
	}
}
