package config

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// setEnv đặt toàn bộ biến mà Load đọc; biến không có trong env được đặt rỗng
// để môi trường của máy chạy test không lọt vào.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range []string{"PORT", "DATABASE_URL", "DB_MAX_CONNS", "LOG_LEVEL", "LLM_PROVIDER", "LLM_API_KEY", "LLM_EMBED_MODEL", "LLM_CHAT_MODEL", "STORAGE_DIR", "ALLOWED_ORIGINS",
		"INGEST_WORKERS", "INGEST_QUEUE", "INGEST_JOB_TIMEOUT", "ASK_TOP_K", "CHUNK_TOKENS", "CHUNK_OVERLAP"} {
		t.Setenv(k, env[k])
	}
}

const testDBURL = "postgres://u:p@localhost:5432/db"

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": testDBURL})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Addr() != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr())
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns = %d, want 10", cfg.DBMaxConns)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want INFO", cfg.LogLevel)
	}
	if cfg.LLMProvider != "mock" {
		t.Errorf("LLMProvider = %q, want mock", cfg.LLMProvider)
	}
	if cfg.StorageDir != "storage" {
		t.Errorf("StorageDir = %q, want storage", cfg.StorageDir)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("AllowedOrigins = %q, want [http://localhost:3000]", cfg.AllowedOrigins)
	}
	// Mặc định giữ đúng các quyết định đã chốt (chunk 500/50, top-5).
	if cfg.IngestWorkers != 4 || cfg.IngestQueue != 100 || cfg.IngestJobTimeout != 10*time.Minute ||
		cfg.AskTopK != 5 || cfg.ChunkTokens != 500 || cfg.ChunkOverlap != 50 {
		t.Errorf("tuning defaults = %d %d %v %d %d %d", cfg.IngestWorkers, cfg.IngestQueue, cfg.IngestJobTimeout,
			cfg.AskTopK, cfg.ChunkTokens, cfg.ChunkOverlap)
	}
}

func TestLoadTuningOverrides(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL": testDBURL, "INGEST_WORKERS": "2", "INGEST_QUEUE": "20", "INGEST_JOB_TIMEOUT": "90s",
		"ASK_TOP_K": "8", "CHUNK_TOKENS": "300", "CHUNK_OVERLAP": "30",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IngestWorkers != 2 || cfg.IngestQueue != 20 || cfg.IngestJobTimeout != 90*time.Second ||
		cfg.AskTopK != 8 || cfg.ChunkTokens != 300 || cfg.ChunkOverlap != 30 {
		t.Errorf("tuning = %+v", cfg)
	}
}

func TestLoadAllowedOrigins(t *testing.T) {
	tests := []struct {
		env     string
		want    []string
		wantErr string
	}{
		{env: " https://app.example.com , http://localhost:3000,", want: []string{"https://app.example.com", "http://localhost:3000"}},
		{env: "*", want: []string{"*"}},
		{env: "https://app.example.com/", wantErr: `"https://app.example.com/"`},
		{env: "app.example.com", wantErr: `"app.example.com"`},
		{env: "ftp://x.com", wantErr: `"ftp://x.com"`},
		{env: " , ", wantErr: "ALLOWED_ORIGINS rỗng"},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			setEnv(t, map[string]string{"DATABASE_URL": testDBURL, "ALLOWED_ORIGINS": tt.env})
			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if strings.Join(cfg.AllowedOrigins, "|") != strings.Join(tt.want, "|") {
				t.Errorf("AllowedOrigins = %q, want %q", cfg.AllowedOrigins, tt.want)
			}
		})
	}
}

func TestLoadPortFromEnv(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": testDBURL, "PORT": "10000"})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr() != ":10000" {
		t.Errorf("Addr = %q, want :10000", cfg.Addr())
	}
}

func TestLoadOverrides(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL": testDBURL,
		"DB_MAX_CONNS": "4",
		"LOG_LEVEL":    "DEBUG",
		"LLM_PROVIDER": "Gemini",
		"LLM_API_KEY":  "k",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBMaxConns != 4 {
		t.Errorf("DBMaxConns = %d, want 4", cfg.DBMaxConns)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want DEBUG", cfg.LogLevel)
	}
	if cfg.LLMProvider != "gemini" {
		t.Errorf("LLMProvider = %q, want gemini", cfg.LLMProvider)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantMsg []string
	}{
		{"missing DATABASE_URL", map[string]string{}, []string{"DATABASE_URL"}},
		{"PORT not a number", map[string]string{"DATABASE_URL": testDBURL, "PORT": "abc"}, []string{"PORT"}},
		{"PORT out of range", map[string]string{"DATABASE_URL": testDBURL, "PORT": "70000"}, []string{"PORT=70000"}},
		{"PORT zero", map[string]string{"DATABASE_URL": testDBURL, "PORT": "0"}, []string{"PORT=0"}},
		{"DB_MAX_CONNS zero", map[string]string{"DATABASE_URL": testDBURL, "DB_MAX_CONNS": "0"}, []string{"DB_MAX_CONNS"}},
		{"bad LOG_LEVEL", map[string]string{"DATABASE_URL": testDBURL, "LOG_LEVEL": "loud"}, []string{"LOG_LEVEL"}},
		{"gemini without key", map[string]string{"DATABASE_URL": testDBURL, "LLM_PROVIDER": "gemini"}, []string{"LLM_API_KEY"}},
		{"unknown provider", map[string]string{"DATABASE_URL": testDBURL, "LLM_PROVIDER": "openai"}, []string{"openai"}},
		{"INGEST_WORKERS zero", map[string]string{"DATABASE_URL": testDBURL, "INGEST_WORKERS": "0"}, []string{"INGEST_WORKERS"}},
		{"INGEST_QUEUE zero", map[string]string{"DATABASE_URL": testDBURL, "INGEST_QUEUE": "0"}, []string{"INGEST_QUEUE"}},
		{"bad INGEST_JOB_TIMEOUT", map[string]string{"DATABASE_URL": testDBURL, "INGEST_JOB_TIMEOUT": "10"}, []string{"INGEST_JOB_TIMEOUT"}},
		{"ASK_TOP_K too big", map[string]string{"DATABASE_URL": testDBURL, "ASK_TOP_K": "100"}, []string{"ASK_TOP_K"}},
		{"CHUNK_TOKENS too small", map[string]string{"DATABASE_URL": testDBURL, "CHUNK_TOKENS": "10"}, []string{"CHUNK_TOKENS"}},
		{"overlap not below chunk", map[string]string{"DATABASE_URL": testDBURL, "CHUNK_TOKENS": "100", "CHUNK_OVERLAP": "100"}, []string{"CHUNK_OVERLAP"}},
		{
			"reports every problem at once",
			map[string]string{"PORT": "x", "LLM_PROVIDER": "openai"},
			[]string{"PORT", "DATABASE_URL", "openai"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)

			_, err := Load()
			if err == nil {
				t.Fatal("Load: want error, got nil")
			}
			for _, msg := range tt.wantMsg {
				if !strings.Contains(err.Error(), msg) {
					t.Errorf("error %q does not mention %q", err, msg)
				}
			}
		})
	}
}

func TestLoadChatModels(t *testing.T) {
	tests := []struct {
		env  string
		want []string
	}{
		{"", nil},
		{"gemini-3.5-flash", []string{"gemini-3.5-flash"}},
		{" a , b,, c ,", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			setEnv(t, map[string]string{"DATABASE_URL": testDBURL, "LLM_CHAT_MODEL": tt.env})
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if fmt.Sprint(cfg.LLMChatModels) != fmt.Sprint(tt.want) || len(cfg.LLMChatModels) != len(tt.want) {
				t.Errorf("LLMChatModels = %q, want %q", cfg.LLMChatModels, tt.want)
			}
		})
	}
}
