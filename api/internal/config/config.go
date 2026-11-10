// Package config đọc cấu hình từ biến môi trường.
package config

import (
	"errors"
	"fmt"
	"log/slog"
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
