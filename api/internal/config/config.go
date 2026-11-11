// Package config đọc cấu hình từ biến môi trường.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Port lấy từ env PORT (Render cấp động), mặc định 8080.
	Port        int
	DatabaseURL string
	DBMaxConns  int32
	LogLevel    slog.Level
	LLMProvider string
	LLMAPIKey   string
	// Model của Gemini; rỗng = mặc định của package llm. LLM_CHAT_MODEL là
	// danh sách phân tách bằng dấu phẩy, thử lần lượt khi model trước hết
	// quota.
	LLMEmbedModel string
	LLMChatModels []string
	// StorageDir chứa file PDF đã tải lên (STORAGE_DIR).
	StorageDir string
	// AllowedOrigins cho CORS, từ ALLOWED_ORIGINS phân tách bằng dấu phẩy.
	AllowedOrigins []string

	// Tinh chỉnh; mặc định là các quyết định đã chốt trong CLAUDE.md.
	IngestWorkers    int           // INGEST_WORKERS: số tài liệu xử lý cùng lúc
	IngestQueue      int           // INGEST_QUEUE: số tài liệu chờ tối đa
	IngestJobTimeout time.Duration // INGEST_JOB_TIMEOUT: thời gian tối đa cho một tài liệu
	AskTopK          int           // ASK_TOP_K: số đoạn đưa vào prompt
	ChunkTokens      int           // CHUNK_TOKENS: độ dài một đoạn
	ChunkOverlap     int           // CHUNK_OVERLAP: phần chồng lấn giữa hai đoạn
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}

func Load() (Config, error) {
	var errs []error

	cfg := Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		LLMProvider:   strings.ToLower(getenv("LLM_PROVIDER", "mock")),
		LLMAPIKey:     os.Getenv("LLM_API_KEY"),
		LLMEmbedModel: os.Getenv("LLM_EMBED_MODEL"),
		LLMChatModels: splitList(os.Getenv("LLM_CHAT_MODEL")),
		StorageDir:    getenv("STORAGE_DIR", "storage"),
	}

	port, err := intEnv("PORT", 8080)
	if err != nil {
		errs = append(errs, err)
	} else if port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PORT=%d ngoài khoảng 1-65535", port))
	}
	cfg.Port = port

	maxConns, err := intEnv("DB_MAX_CONNS", 10)
	if err != nil {
		errs = append(errs, err)
	} else if maxConns < 1 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS phải >= 1, nhận %d", maxConns))
	}
	cfg.DBMaxConns = int32(maxConns)

	if err := cfg.LogLevel.UnmarshalText([]byte(getenv("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	origins, err := parseOrigins(getenv("ALLOWED_ORIGINS", "http://localhost:3000"))
	if err != nil {
		errs = append(errs, err)
	}
	cfg.AllowedOrigins = origins

	errs = append(errs, cfg.loadTuning()...)

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("thiếu DATABASE_URL"))
	}

	switch cfg.LLMProvider {
	case "mock":
	case "gemini":
		if cfg.LLMAPIKey == "" {
			errs = append(errs, errors.New("LLM_PROVIDER=gemini cần LLM_API_KEY"))
		}
	default:
		errs = append(errs, fmt.Errorf("LLM_PROVIDER không hỗ trợ: %q (mock|gemini)", cfg.LLMProvider))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// loadTuning đọc các thông số tinh chỉnh và trả mọi lỗi cùng lúc.
func (c *Config) loadTuning() []error {
	var errs []error
	intIn := func(key string, def, lo, hi int) int {
		n, err := intEnv(key, def)
		switch {
		case err != nil:
			errs = append(errs, err)
		case n < lo || n > hi:
			errs = append(errs, fmt.Errorf("%s=%d ngoài khoảng %d-%d", key, n, lo, hi))
		}
		return n
	}
	c.IngestWorkers = intIn("INGEST_WORKERS", 4, 1, 64)
	c.IngestQueue = intIn("INGEST_QUEUE", 100, 1, 10000)
	c.AskTopK = intIn("ASK_TOP_K", 5, 1, 20)
	c.ChunkTokens = intIn("CHUNK_TOKENS", 500, 50, 4000)
	c.ChunkOverlap = intIn("CHUNK_OVERLAP", 50, 0, 4000)
	if c.ChunkOverlap >= c.ChunkTokens {
		errs = append(errs, fmt.Errorf("CHUNK_OVERLAP=%d phải nhỏ hơn CHUNK_TOKENS=%d", c.ChunkOverlap, c.ChunkTokens))
	}

	timeout, err := time.ParseDuration(getenv("INGEST_JOB_TIMEOUT", "10m"))
	if err != nil || timeout <= 0 {
		errs = append(errs, fmt.Errorf("INGEST_JOB_TIMEOUT=%q phải là thời lượng dương, vd. 10m", os.Getenv("INGEST_JOB_TIMEOUT")))
	}
	c.IngestJobTimeout = timeout
	return errs
}

// parseOrigins nhận "*" hoặc các origin dạng scheme://host[:port]. Origin có
// đường dẫn hay dấu "/" cuối không bao giờ khớp header Origin, nên báo lỗi
// thay vì để CORS hỏng âm thầm.
func parseOrigins(v string) ([]string, error) {
	var out []string
	var errs []error
	for _, o := range strings.Split(v, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o != "*" {
			u, err := url.Parse(o)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" {
				errs = append(errs, fmt.Errorf("ALLOWED_ORIGINS: %q không phải origin dạng http(s)://host[:port]", o))
				continue
			}
		}
		out = append(out, o)
	}
	if len(out) == 0 && len(errs) == 0 {
		errs = append(errs, errors.New("ALLOWED_ORIGINS rỗng"))
	}
	return out, errors.Join(errs...)
}

// splitList tách chuỗi phân tách bằng dấu phẩy, bỏ khoảng trắng và phần rỗng.
func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("%s=%q không phải số nguyên: %w", key, v, err)
	}
	return n, nil
}
