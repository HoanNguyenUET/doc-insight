package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/internal/config"
	"github.com/HoanNguyenUET/doc-insight/api/internal/handler"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
	"github.com/HoanNguyenUET/doc-insight/api/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	db, err := repository.NewDB(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx, migrations.FS); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	router := handler.NewRouter(handler.Deps{
		Health: handler.NewHealthHandler(service.NewHealthService(db)),
	})

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// Không đặt WriteTimeout: các endpoint hỏi đáp sẽ stream câu trả lời lâu.
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", srv.Addr, "llm_provider", cfg.LLMProvider)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
