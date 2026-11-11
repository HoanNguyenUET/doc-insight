package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

const askTestDocID = "22222222-2222-2222-2222-222222222222"

// fakeAskService chạy run với writer của handler, thay cho luồng embed →
// search → LLM thật.
type fakeAskService struct {
	run   func(ctx context.Context, w service.AnswerWriter) error
	gotID string
	gotQ  string
}

func (f *fakeAskService) Ask(ctx context.Context, documentID, question string, w service.AnswerWriter) error {
	f.gotID, f.gotQ = documentID, question
	return f.run(ctx, w)
}

func askRouter(svc AskService) http.Handler {
	return NewRouter(Deps{
		Health:    NewHealthHandler(service.NewHealthService(pingFunc(func(context.Context) error { return nil }))),
		Documents: NewDocumentHandler(&fakeDocService{}),
		Ask:       NewAskHandler(svc),
	})
}

func askURL(q string) string {
	return "/api/documents/" + askTestDocID + "/ask?q=" + url.QueryEscape(q)
}

type sseEvent struct {
	name string
	data map[string]any
}

// parseSSE tách body thành các event "event: X\ndata: {...}\n\n".
func parseSSE(t *testing.T, body string) []sseEvent {
	t.Helper()
	var events []sseEvent
	for _, block := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		if block == "" {
			continue
		}
		name, data, ok := strings.Cut(block, "\ndata: ")
		if !ok || !strings.HasPrefix(name, "event: ") || strings.Contains(data, "\n") {
			t.Fatalf("malformed SSE block %q", block)
		}
		var ev sseEvent
		ev.name = strings.TrimPrefix(name, "event: ")
		if err := json.Unmarshal([]byte(data), &ev.data); err != nil {
			t.Fatalf("event %s data %q: %v", ev.name, data, err)
		}
		events = append(events, ev)
	}
	return events
}

func eventNames(events []sseEvent) string {
	names := make([]string, len(events))
	for i, e := range events {
		names[i] = e.name
	}
	return strings.Join(names, ",")
}

func streamAnswer(sources []service.Source, tokens ...string) func(context.Context, service.AnswerWriter) error {
	return func(_ context.Context, w service.AnswerWriter) error {
		if err := w.WriteSources(sources); err != nil {
			return err
		}
		for _, tok := range tokens {
			if err := w.WriteToken(tok); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestAskStreamsSSE(t *testing.T) {
	long := strings.Repeat("Nghỉ phép  12 ngày.\n", 20)
	svc := &fakeAskService{run: streamAnswer([]service.Source{
		{N: 1, ChunkID: 9, Page: 7, CharStart: 120, CharEnd: 480, Content: long, Score: 0.87321},
		{N: 2, Page: 3, Content: "Ngắn.", Score: 0.5},
	}, "Theo ", "tài liệu", " [[1]]")}

	rec, _ := do(askRouter(svc), httptest.NewRequest(http.MethodGet, askURL("nghỉ phép?"), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
	if svc.gotID != askTestDocID || svc.gotQ != "nghỉ phép?" {
		t.Errorf("service got (%q, %q)", svc.gotID, svc.gotQ)
	}

	events := parseSSE(t, rec.Body.String())
	if got := eventNames(events); got != "sources,token,token,token,done" {
		t.Fatalf("events = %s", got)
	}

	chunks := events[0].data["chunks"].([]any)
	first := chunks[0].(map[string]any)
	want := map[string]any{"ref": 1.0, "page": 7.0, "char_start": 120.0, "char_end": 480.0, "score": 0.87}
	for k, v := range want {
		if first[k] != v {
			t.Errorf("chunks[0].%s = %v, want %v", k, first[k], v)
		}
	}
	if len(first) != len(want)+1 {
		t.Errorf("chunks[0] = %v, want only %d fields + preview", first, len(want))
	}
	p := first["preview"].(string)
	if n := len([]rune(p)); n > previewRunes+1 || n < previewRunes-10 ||!strings.HasSuffix(p, "…") || strings.Contains(p, "  ") || strings.Contains(p, "\n") {
		t.Errorf("preview (%d runes) = %q, want %d runes of collapsed whitespace + …", n, p, previewRunes)
	}
	if got := chunks[1].(map[string]any)["preview"]; got != "Ngắn." {
		t.Errorf("short preview = %q, want it unchanged", got)
	}

	var text string
	for _, e := range events[1:4] {
		text += e.data["text"].(string)
	}
	if text != "Theo tài liệu [[1]]" {
		t.Errorf("tokens joined = %q", text)
	}
	done := events[4].data
	if done["tokens"] != 3.0 || done["ms"] == nil {
		t.Errorf("done = %v, want tokens 3 and ms", done)
	}
}

func TestAskEmptySourcesIsJSONArray(t *testing.T) {
	svc := &fakeAskService{run: streamAnswer([]service.Source{}, "Không biết.")}
	rec, _ := do(askRouter(svc), httptest.NewRequest(http.MethodGet, askURL("q"), nil))
	if !strings.HasPrefix(rec.Body.String(), "event: sources\ndata: {\"chunks\":[]}\n\n") {
		t.Errorf("body = %q, want sources with chunks []", rec.Body)
	}
}

func TestAskErrorBeforeStreamIsJSON(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		msg    string
	}{
		{"invalid question", fmt.Errorf("%w: câu hỏi trống", service.ErrInvalidQuestion), http.StatusBadRequest, "câu hỏi không hợp lệ: câu hỏi trống"},
		{"not found", service.ErrDocNotFound, http.StatusNotFound, service.ErrDocNotFound.Error()},
		{"not ready", fmt.Errorf("%w (trạng thái embedding)", service.ErrDocNotReady), http.StatusConflict, "tài liệu chưa xử lý xong (trạng thái embedding)"},
		{"embed failed", errors.New("embed câu hỏi: hết quota"), http.StatusInternalServerError, "lỗi máy chủ nội bộ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAskService{run: func(context.Context, service.AnswerWriter) error { return tt.err }}
			rec, body := do(askRouter(svc), httptest.NewRequest(http.MethodGet, askURL(""), nil))
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want JSON", ct)
			}
			if body["error"] != tt.msg || body["request_id"] == "" {
				t.Errorf("body = %v, want error %q with request_id", body, tt.msg)
			}
		})
	}
}

func TestAskErrorMidStreamIsEvent(t *testing.T) {
	svc := &fakeAskService{run: func(ctx context.Context, w service.AnswerWriter) error {
		if err := streamAnswer([]service.Source{{N: 1, Page: 1}}, "Theo ")(ctx, w); err != nil {
			return err
		}
		return errors.New("stream câu trả lời: gemini 503 secret-detail")
	}}
	rec, _ := do(askRouter(svc), httptest.NewRequest(http.MethodGet, askURL("q"), nil))

	events := parseSSE(t, rec.Body.String())
	if got := eventNames(events); got != "sources,token,error" {
		t.Fatalf("events = %s, want error event and no done", got)
	}
	e := events[2].data
	if e["message"] != "lỗi máy chủ nội bộ" || e["request_id"] == "" {
		t.Errorf("error event = %v, want generic message (no internals) with request_id", e)
	}
}

func TestAskClientGoneStopsWriting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var tokenErr error
	svc := &fakeAskService{run: func(_ context.Context, w service.AnswerWriter) error {
		if err := w.WriteSources(nil); err != nil {
			return err
		}
		cancel()
		tokenErr = w.WriteToken("x")
		return tokenErr
	}}
	req := httptest.NewRequest(http.MethodGet, askURL("q"), nil).WithContext(ctx)
	rec, _ := do(askRouter(svc), req)

	if !errors.Is(tokenErr, context.Canceled) {
		t.Errorf("WriteToken after disconnect = %v, want context.Canceled", tokenErr)
	}
	if got := eventNames(parseSSE(t, rec.Body.String())); got != "sources" {
		t.Errorf("events = %s, want nothing after the client left", got)
	}
}

// Qua server HTTP thật và toàn bộ middleware: event sources phải tới client
// trong khi câu trả lời còn đang sinh, tức là Flush đi xuyên qua wrapper của
// requestLogger.
func TestAskFlushesEachEvent(t *testing.T) {
	release := make(chan struct{})
	svc := &fakeAskService{run: func(ctx context.Context, w service.AnswerWriter) error {
		if err := w.WriteSources([]service.Source{{N: 1, Page: 7}}); err != nil {
			return err
		}
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return w.WriteToken("xong")
	}}
	srv := httptest.NewServer(askRouter(svc))
	defer srv.Close()

	resp, err := http.Get(srv.URL + askURL("q"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	readLine := func() string {
		select {
		case l := <-lines:
			return l
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for a flushed line")
			return ""
		}
	}

	if l := readLine(); l != "event: sources" {
		t.Fatalf("first line = %q", l)
	}
	close(release)
	var rest []string
	for l := range lines {
		rest = append(rest, l)
	}
	if got := strings.Join(rest, "\n"); !strings.Contains(got, "event: token\ndata: {\"text\":\"xong\"}") || !strings.Contains(got, "event: done") {
		t.Errorf("rest of stream = %q", got)
	}
}
