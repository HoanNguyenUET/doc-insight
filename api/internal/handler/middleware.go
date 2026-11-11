package handler

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requestIDHeader trả lại request ID cho client để báo lỗi kèm mã tra log.
func requestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(middleware.RequestIDHeader, middleware.GetReqID(r.Context()))
		next.ServeHTTP(w, r)
	})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		level := slog.LevelInfo
		if r.URL.Path == "/healthz" && ww.Status() < 500 {
			// Render gọi healthz liên tục; hạ mức để không làm nhiễu log.
			level = slog.LevelDebug
		}
		slog.Log(r.Context(), level, "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

// recoverer bắt panic, ghi log JSON kèm stack và request_id, trả 500.
// Đặt sau requestLogger để dòng log request thấy status 500.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec) // net/http dùng panic này để huỷ response; không nuốt
			}
			slog.ErrorContext(r.Context(), "panic in handler", "panic", rec, "stack", string(debug.Stack()),
				"request_id", middleware.GetReqID(r.Context()))
			writeJSON(w, http.StatusInternalServerError, errorBody{Error: "lỗi máy chủ nội bộ", RequestID: middleware.GetReqID(r.Context())})
		}()
		next.ServeHTTP(w, r)
	})
}

// cors cho phép các origin trong danh sách ("*" là mọi origin). Không dùng
// cookie nên không bật Allow-Credentials.
func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			h := w.Header()
			ok := origin != "" && (allowAll || allowed[origin])
			if origin != "" {
				h.Add("Vary", "Origin")
			}
			if ok {
				h.Set("Access-Control-Allow-Origin", origin)
				// Accept-Ranges/Content-Range: pdf.js cần đọc để tải PDF theo khúc.
				h.Set("Access-Control-Expose-Headers", middleware.RequestIDHeader+", Accept-Ranges, Content-Range, Content-Length")
			}

			// Preflight: trả lời luôn, không đi tiếp vào router (router sẽ 405).
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				if ok {
					h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Content-Type, Range, "+middleware.RequestIDHeader)
					h.Set("Access-Control-Max-Age", "600")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
