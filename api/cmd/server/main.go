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
	"github.com/HoanNguyenUET/doc-insight/api/internal/ingest"
	"github.com/HoanNguyenUET/doc-insight/api/internal/llm"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
	"github.com/HoanNguyenUET/doc-insight/api/migrations"
)

const (
	ingestWorkers    = 4
	ingestQueueSize  = 100
	ingestJobTimeout = 10 * time.Minute
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
	// Bật log JSON trước khi đọc config để cả lỗi cấu hình cũng ra JSON.
	logLevel := new(slog.LevelVar)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logLevel.Set(cfg.LogLevel)

	db, err := repository.NewDB(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx, migrations.FS); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if err := os.MkdirAll(cfg.StorageDir, 0o750); err != nil {
		return fmt.Errorf("create STORAGE_DIR: %w", err)
	}

	// Tắt êm thì pool tự ghi lỗi cho job dở; còn nếu lần chạy trước bị kill
	// thì tài liệu treo ở trạng thái đang xử lý mãi. Giả định chỉ một instance
	// xử lý ingest cho mỗi database.
	if n, err := db.FailUnfinishedDocuments(ctx, "server dừng đột ngột khi đang xử lý; hãy tải lên lại"); err != nil {
		return fmt.Errorf("recover unfinished documents: %w", err)
	} else if n > 0 {
		slog.Warn("marked unfinished documents as failed", "count", n)
	}

	// Pool dùng context riêng: chỉ dừng sau khi HTTP server đã ngừng nhận
	// upload, và phải dừng trước khi đóng DB (defer chạy ngược thứ tự).
	provider := newLLMProvider(cfg)
	pool := ingest.NewPool(ingest.NewPipeline(db, provider), ingestWorkers, ingestQueueSize, ingestJobTimeout)
	poolCtx, stopPool := context.WithCancel(context.WithoutCancel(ctx))
	pool.Start(poolCtx)
	defer func() {
		stopPool()
		pool.Wait()
		slog.Info("ingest workers stopped")
	}()

	router := handler.NewRouter(handler.Deps{
		Health:         handler.NewHealthHandler(service.NewHealthService(db)),
		Documents:      handler.NewDocumentHandler(service.NewDocumentService(db, pool, cfg.StorageDir)),
		Ask:            handler.NewAskHandler(service.NewAskService(db, provider)),
		AllowedOrigins: cfg.AllowedOrigins,
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
		slog.Info("http server listening", "addr", srv.Addr, "llm_provider", cfg.LLMProvider,
			"storage_dir", cfg.StorageDir, "allowed_origins", cfg.AllowedOrigins, "ingest_workers", ingestWorkers)
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

// newLLMProvider chọn provider theo LLM_PROVIDER (config.Load đã kiểm tra
// giá trị và LLM_API_KEY).
func newLLMProvider(cfg config.Config) service.LLMProvider {
	if cfg.LLMProvider == "gemini" {
		return llm.NewGeminiProvider(cfg.LLMAPIKey, cfg.LLMEmbedModel, cfg.LLMChatModel)
	}
	return llm.NewMockProvider()
}
