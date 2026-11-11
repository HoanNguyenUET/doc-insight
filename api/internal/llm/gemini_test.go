package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestGemini trỏ provider vào server giả; backoff 1ms để test retry nhanh.
func newTestGemini(t *testing.T, h http.HandlerFunc) *GeminiProvider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	g := NewGeminiProvider("test-key", "", nil)
	g.baseURL = srv.URL
	g.backoff = time.Millisecond
	g.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	return g
}

func vector(fill float32) []float32 {
	v := make([]float32, Dimensions)
	for i := range v {
		v[i] = fill
	}
	return v
}

func embedHandler(t *testing.T, batchSizes *[]int, dims int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("api key header = %q", got)
		}
		if r.URL.Query().Has("key") {
			t.Error("api key must not be in the query string")
		}
		if r.URL.Path != "/models/gemini-embedding-001:batchEmbedContents" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var req struct {
			Requests []geminiEmbedRequest `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		*batchSizes = append(*batchSizes, len(req.Requests))
		var res geminiEmbedResponse
		for _, rq := range req.Requests {
			if rq.Model != "models/gemini-embedding-001" || rq.OutputDimensionality != Dimensions {
				t.Errorf("request = %+v", rq)
			}
			// Mã hoá văn bản "t<i>" thành giá trị để kiểm tra thứ tự.
			var i int
			fmt.Sscanf(rq.Content.Parts[0].Text, "t%d", &i)
			v := vector(float32(i))[:dims]
			res.Embeddings = append(res.Embeddings, struct {
				Values []float32 `json:"values"`
			}{v})
		}
		json.NewEncoder(w).Encode(res)
	}
}

func TestGeminiEmbed(t *testing.T) {
	tests := []struct {
		name        string
		n           int
		dims        int
		wantBatches []int
		wantErr     string
	}{
		{"one batch", 3, Dimensions, []int{3}, ""},
		{"split at 100", 250, Dimensions, []int{100, 100, 50}, ""},
		{"empty input", 0, Dimensions, nil, ""},
		{"wrong dimensions", 2, 512, []int{2}, "cần 768"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var batches []int
			g := newTestGemini(t, embedHandler(t, &batches, tt.dims))
			texts := make([]string, tt.n)
			for i := range texts {
				texts[i] = fmt.Sprintf("t%d", i)
			}
			vecs, err := g.Embed(context.Background(), texts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want dimension error", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(batches) != fmt.Sprint(tt.wantBatches) {
				t.Errorf("batches = %v, want %v", batches, tt.wantBatches)
			}
			if len(vecs) != tt.n {
				t.Fatalf("got %d vectors, want %d", len(vecs), tt.n)
			}
			for i, v := range vecs {
				if v[0] != float32(i) {
					t.Fatalf("vector %d out of order: starts with %v", i, v[0])
				}
			}
		})
	}
}

func TestGeminiRetry(t *testing.T) {
	tests := []struct {
		name         string
		statuses     []int // status của từng lần gọi; hết danh sách thì trả 200
		wantAttempts int32
		wantStatus   int // 0 = thành công
	}{
		{"429 then ok", []int{429}, 2, 0},
		{"5xx three times then ok", []int{500, 502, 503}, 4, 0},
		{"gives up after 3 retries", []int{503, 503, 503, 503, 503}, 4, 503},
		{"400 is not retried", []int{400}, 1, 400},
		{"403 is not retried", []int{403}, 1, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
				n := int(attempts.Add(1))
				if n <= len(tt.statuses) {
					w.WriteHeader(tt.statuses[n-1])
					fmt.Fprintf(w, `{"error":{"code":%d,"message":"lỗi giả %d"}}`, tt.statuses[n-1], n)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"embeddings": []map[string]any{{"values": vector(0.1)}}})
			})

			_, err := g.Embed(context.Background(), []string{"x"})
			if got := attempts.Load(); got != tt.wantAttempts {
				t.Errorf("attempts = %d, want %d", got, tt.wantAttempts)
			}
			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.wantStatus || !strings.Contains(apiErr.Message, "lỗi giả") {
				t.Errorf("err = %v, want APIError %d with server message", err, tt.wantStatus)
			}
		})
	}
}

func TestGeminiRetryStopsOnCancel(t *testing.T) {
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := g.Embed(ctx, []string{"x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("returned after %v; must not wait for Retry-After once ctx is done", d)
	}
}

func TestGeminiTimeout(t *testing.T) {
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		// Đọc hết body thì server mới phát hiện được client ngắt kết nối.
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	g.timeout = 20 * time.Millisecond
	_, err := g.Embed(context.Background(), []string{"x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func sse(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: " + e + "\r\n\r\n")
	}
	return b.String()
}

func TestGeminiStream(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		want       []string
		wantErr    error
		wantErrMsg string
	}{
		{
			name: "text parts in order, thoughts skipped",
			body: sse(
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"Nghỉ phép "}]}}]}`,
				`{"candidates":[{"content":{"parts":[{"text":"suy nghĩ","thought":true},{"text":"12 ngày "},{"text":"[[1]]"}]}}]}`,
				`{"candidates":[{"content":{"parts":[{"text":"."}]},"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":9}}`,
			),
			want: []string{"Nghỉ phép ", "12 ngày ", "[[1]]", "."},
		},
		{
			name:    "blocked prompt",
			body:    sse(`{"promptFeedback":{"blockReason":"SAFETY"}}`),
			wantErr: ErrBlocked,
		},
		{
			name:    "answer stopped by safety after partial text",
			body:    sse(`{"candidates":[{"content":{"parts":[{"text":"Một phần"}]},"finishReason":"SAFETY"}]}`),
			want:    []string{"Một phần"},
			wantErr: ErrBlocked,
		},
		{
			name:       "error object mid-stream",
			body:       sse(`{"candidates":[{"content":{"parts":[{"text":"A"}]}}]}`, `{"error":{"code":500,"message":"nội bộ"}}`),
			want:       []string{"A"},
			wantErrMsg: "nội bộ",
		},
		{
			name:       "malformed json",
			body:       sse(`{"candidates":`),
			wantErrMsg: "decode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/models/gemini-3.5-flash:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" {
					t.Errorf("url = %s", r.URL)
				}
				var req struct {
					Contents []geminiContent `json:"contents"`
				}
				json.NewDecoder(r.Body).Decode(&req)
				if len(req.Contents) != 1 || req.Contents[0].Parts[0].Text != "prompt" {
					t.Errorf("request contents = %+v", req.Contents)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, tt.body)
			})

			out := make(chan string, 100)
			err := g.Stream(context.Background(), "prompt", out)
			close(out)
			var got []string
			for s := range out {
				got = append(got, s)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("tokens = %q, want %q", got, tt.want)
			}
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.wantErrMsg != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("err = %v, want containing %q", err, tt.wantErrMsg)
				}
			case err != nil:
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestGeminiStreamRetriesBeforeFirstToken(t *testing.T) {
	var attempts atomic.Int32
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, sse(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	})
	out := make(chan string, 10)
	if err := g.Stream(context.Background(), "prompt", out); err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 || len(out) != 1 {
		t.Errorf("attempts = %d, tokens = %d; want 2 attempts and 1 token", attempts.Load(), len(out))
	}
}

func TestGeminiStreamStopsWhenReaderGone(t *testing.T) {
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 100; i++ {
			io.WriteString(w, sse(`{"candidates":[{"content":{"parts":[{"text":"x"}]}}]}`))
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan string) // không ai đọc
	errc := make(chan error, 1)
	go func() { errc <- g.Stream(ctx, "prompt", out) }()
	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream blocked on send after ctx was cancelled")
	}
}

// quotaBody giống response 429 thật của Gemini: thời gian chờ nằm trong
// details (google.rpc.RetryInfo), không có header Retry-After.
func quotaBody(delay string) string {
	return `{"error":{"code":429,"message":"You exceeded your current quota","status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaMetric":"embed_content_free_tier_requests"}]},` +
		`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"` + delay + `"}]}}`
}

func TestReadAPIErrorRetryDelay(t *testing.T) {
	tests := []struct {
		name string
		body string
		want time.Duration
	}{
		{"seconds with fraction", quotaBody("56.908261311s"), 56908261311 * time.Nanosecond},
		{"whole seconds", quotaBody("3s"), 3 * time.Second},
		{"no details", `{"error":{"code":429,"message":"x"}}`, 0},
		{"bad duration", quotaBody("soon"), 0},
		{"not json", `rate limited`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusTooManyRequests)
			rec.WriteString(tt.body)
			if got := readAPIError(rec.Result()).RetryDelay; got != tt.want {
				t.Errorf("RetryDelay = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackoffFor(t *testing.T) {
	g := NewGeminiProvider("k", "", nil)
	tests := []struct {
		name        string
		attempt     int
		retryAfter  string
		serverDelay time.Duration
		min, max    time.Duration
	}{
		{"exponential", 2, "", 0, 2 * time.Second, 3 * time.Second},
		{"exponential capped", 10, "", 0, geminiMaxBackoff, geminiMaxBackoff},
		{"Retry-After header", 0, "7", 0, 7 * time.Second, 7 * time.Second},
		// Quota theo phút của Gemini hay đòi chờ gần 60s: phải chờ đủ, không cắt ở 30s.
		{"RetryInfo above exponential cap", 0, "", 57 * time.Second, 57 * time.Second, 57 * time.Second},
		{"larger of header and RetryInfo", 0, "40", 20 * time.Second, 40 * time.Second, 40 * time.Second},
		{"server delay capped", 0, "", time.Hour, geminiMaxServerDelay, geminiMaxServerDelay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.backoffFor(tt.attempt, tt.retryAfter, tt.serverDelay)
			if got < tt.min || got > tt.max {
				t.Errorf("backoffFor = %v, want in [%v, %v]", got, tt.min, tt.max)
			}
		})
	}
}

func TestGeminiRetryHonorsRetryInfo(t *testing.T) {
	var attempts atomic.Int32
	var first time.Time
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			first = time.Now()
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, quotaBody("0.2s"))
			return
		}
		if waited := time.Since(first); waited < 200*time.Millisecond {
			t.Errorf("retried after %v, server asked for 200ms", waited)
		}
		json.NewEncoder(w).Encode(map[string]any{"embeddings": []map[string]any{{"values": vector(0.1)}}})
	})
	if _, err := g.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if n := attempts.Load(); n != 2 {
		t.Errorf("attempts = %d, want 2", n)
	}
}

// Chờ quá deadline của ctx là vô ích: trả lỗi ngay, giữ lỗi 429 để biết lý do.
func TestGeminiRetryGivesUpWhenDelayExceedsDeadline(t *testing.T) {
	var attempts atomic.Int32
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, quotaBody("30s"))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	_, err := g.Embed(ctx, []string{"x"})
	if d := time.Since(start); d > time.Second {
		t.Errorf("returned after %v, want immediately", d)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("err = %v, want APIError 429", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1", n)
	}
}

// Gemini thỉnh thoảng treo không trả header; chưa đọc gì nên gọi lại là an toàn.
func TestGeminiRetriesTimeout(t *testing.T) {
	var attempts atomic.Int32
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if attempts.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"embeddings": []map[string]any{{"values": vector(0.1)}}})
	})
	g.timeout = 50 * time.Millisecond
	if _, err := g.Embed(context.Background(), []string{"x"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if n := attempts.Load(); n != 2 {
		t.Errorf("attempts = %d, want 2", n)
	}
}

func TestGeminiDoesNotRetryWhenCallerCancelled(t *testing.T) {
	var attempts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		attempts.Add(1)
		cancel()
		<-r.Context().Done()
	})
	_, err := g.Embed(ctx, []string{"x"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want Canceled", err)
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1", n)
	}
}

// Quota theo ngày đòi chờ hàng giờ: chờ cũng vô ích, báo lỗi ngay.
func TestGeminiDoesNotRetryLongRetryInfo(t *testing.T) {
	var attempts atomic.Int32
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, quotaBody("72387s"))
	})
	start := time.Now()
	_, err := g.Embed(context.Background(), []string{"x"})
	if d := time.Since(start); d > time.Second {
		t.Errorf("returned after %v, want immediately", d)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("err = %v, want APIError 429", err)
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1", n)
	}
}

// fallbackServer trả status/body cố định theo model trong URL; model không có
// trong map thì stream "từ <model>".
func fallbackServer(t *testing.T, fail map[string]func(w http.ResponseWriter), calls *[]string) *GeminiProvider {
	t.Helper()
	var mu sync.Mutex
	g := newTestGemini(t, func(w http.ResponseWriter, r *http.Request) {
		model := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/models/"), ":streamGenerateContent")
		mu.Lock()
		*calls = append(*calls, model)
		mu.Unlock()
		if f, ok := fail[model]; ok {
			f(w)
			return
		}
		io.WriteString(w, sse(`{"candidates":[{"content":{"parts":[{"text":"từ `+model+`"}]}}]}`))
	})
	g.chatModels = []string{"a", "b", "c"}
	return g
}

func status(code int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
		io.WriteString(w, body)
	}
}

func TestGeminiStreamFallback(t *testing.T) {
	daily := status(http.StatusTooManyRequests, quotaBody("72387s"))
	tests := []struct {
		name       string
		fail       map[string]func(w http.ResponseWriter)
		wantText   string
		wantCalls  string
		wantStatus int // 0 = thành công
	}{
		{"first model ok", nil, "từ a", "[a]", 0},
		{"daily quota falls back", map[string]func(http.ResponseWriter){"a": daily}, "từ b", "[a b]", 0},
		// Còn model khác thì không chờ quota theo phút, chuyển ngay.
		{"per-minute quota falls back without waiting", map[string]func(http.ResponseWriter){
			"a": status(http.StatusTooManyRequests, quotaBody("50s")),
		}, "từ b", "[a b]", 0},
		{"model gone falls back", map[string]func(http.ResponseWriter){"a": status(http.StatusNotFound, `{"error":{"code":404,"message":"not found"}}`)}, "từ b", "[a b]", 0},
		{"two models down", map[string]func(http.ResponseWriter){"a": daily, "b": daily}, "từ c", "[a b c]", 0},
		{"bad request does not fall back", map[string]func(http.ResponseWriter){
			"a": status(http.StatusBadRequest, `{"error":{"code":400,"message":"sai"}}`),
		}, "", "[a]", http.StatusBadRequest},
		{"all exhausted", map[string]func(http.ResponseWriter){"a": daily, "b": daily, "c": daily}, "", "[a b c]", http.StatusTooManyRequests},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			g := fallbackServer(t, tt.fail, &calls)
			out := make(chan string, 10)
			start := time.Now()
			err := g.Stream(context.Background(), "prompt", out)
			close(out)
			var text string
			for s := range out {
				text += s
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v, must not wait on quota while another model is left", d)
			}
			if text != tt.wantText || fmt.Sprint(calls) != tt.wantCalls {
				t.Errorf("text = %q calls = %v, want %q %s", text, calls, tt.wantText, tt.wantCalls)
			}
			var apiErr *APIError
			switch {
			case tt.wantStatus == 0 && err != nil:
				t.Errorf("err = %v", err)
			case tt.wantStatus != 0 && (!errors.As(err, &apiErr) || apiErr.StatusCode != tt.wantStatus):
				t.Errorf("err = %v, want APIError %d", err, tt.wantStatus)
			}
		})
	}
}

// Model hết quota ngày được bỏ qua ở các câu hỏi sau, không tốn thêm request.
func TestGeminiStreamSkipsExhaustedModel(t *testing.T) {
	var calls []string
	g := fallbackServer(t, map[string]func(w http.ResponseWriter){
		"a": status(http.StatusTooManyRequests, quotaBody("72387s")),
	}, &calls)
	for range 2 {
		if err := g.Stream(context.Background(), "prompt", make(chan string, 10)); err != nil {
			t.Fatal(err)
		}
	}
	if fmt.Sprint(calls) != "[a b b]" {
		t.Errorf("calls = %v, want [a b b]", calls)
	}

	// Hết thời gian chờ thì thử lại model đầu.
	g.now = func() time.Time { return time.Now().Add(73000 * time.Second) }
	calls = nil
	g.Stream(context.Background(), "prompt", make(chan string, 10))
	if fmt.Sprint(calls) != "[a b]" {
		t.Errorf("after cooldown calls = %v, want [a b]", calls)
	}
}

func TestAPIErrorQuotaExhausted(t *testing.T) {
	for _, tt := range []struct {
		status int
		want   bool
	}{{429, true}, {500, false}, {400, false}} {
		if got := (&APIError{StatusCode: tt.status}).QuotaExhausted(); got != tt.want {
			t.Errorf("status %d: QuotaExhausted = %v, want %v", tt.status, got, tt.want)
		}
	}
}
