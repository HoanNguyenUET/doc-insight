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
)

type Config struct {
	// Port lấy từ env PORT (Render cấp động), mặc định 8080.
	Port        int
	DatabaseURL string
	DBMaxConns  int32
	LogLevel    slog.Level
	LLMProvider string
	LLMAPIKey   string
	// StorageDir chứa file PDF đã tải lên (STORAGE_DIR).
	StorageDir string
	// AllowedOrigins cho CORS, từ ALLOWED_ORIGINS phân tách bằng dấu phẩy.
	AllowedOrigins []string
}

func (c Config) Addr() string {
	return ":" + strconv.Itoa(c.Port)
}

func Load() (Config, error) {
	var errs []error

	cfg := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LLMProvider: strings.ToLower(getenv("LLM_PROVIDER", "mock")),
		LLMAPIKey:   os.Getenv("LLM_API_KEY"),
		StorageDir:  getenv("STORAGE_DIR", "storage"),
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
