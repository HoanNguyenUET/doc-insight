package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// text-embedding-004 và gemini-2.0-flash đã bị Google tắt trong năm 2026.
	// Không dùng gemini-embedding-2: nó gộp mọi văn bản của một request thành
	// một vector, trong khi mỗi chunk cần vector riêng.
	DefaultGeminiEmbedModel = "gemini-embedding-001"

	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	// geminiTimeout áp cho từng lần gọi, kể cả thời gian đọc hết stream.
	geminiTimeout     = 60 * time.Second
	geminiMaxRetries  = 3
	geminiBaseBackoff = 500 * time.Millisecond
	geminiMaxBackoff  = 30 * time.Second
	// Thời gian chờ do server yêu cầu (Retry-After, RetryInfo) được chờ đủ,
	// vì quota theo phút của gói miễn phí thường đòi gần 60s. RetryInfo đòi
	// lâu hơn mức này (quota theo ngày) thì báo lỗi ngay, không chờ.
	geminiMaxServerDelay = 2 * time.Minute
	// Giới hạn số văn bản của một lần batchEmbedContents.
	geminiMaxEmbedBatch = 100
	// Một dòng SSE chứa trọn một JSON response; 1 MB là dư.
	geminiMaxSSELine = 1 << 20
)

// DefaultGeminiChatModels thử lần lượt: quota miễn phí tính riêng từng model,
// nên khi model tốt nhất hết lượt trong ngày thì rơi xuống bản lite.
var DefaultGeminiChatModels = []string{"gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-3.1-flash-lite"}

// ErrBlocked: Gemini từ chối prompt hoặc dừng câu trả lời vì bộ lọc an toàn.
var ErrBlocked = errors.New("gemini chặn nội dung")

// APIError là response lỗi (không phải 200) của Gemini API.
type APIError struct {
	StatusCode int
	Message    string
	// RetryDelay là thời gian server bảo chờ trước khi gọi lại (RetryInfo
	// trong details); 0 nếu không có.
	RetryDelay time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gemini: HTTP %d: %s", e.StatusCode, e.Message)
}

type GeminiProvider struct {
	apiKey     string
	embedModel string
	// chatModels thử theo thứ tự khi model trước hết quota hoặc không còn.
	// Embedding chỉ có một model: vector của hai model khác nhau không so
	// sánh được với nhau.
	chatModels []string
	baseURL    string
	client     *http.Client
	timeout    time.Duration
	maxRetries int
	backoff    time.Duration
	log        *slog.Logger
	now        func() time.Time

	mu sync.Mutex
	// cooldown[model]: chưa tới thời điểm này thì bỏ qua model (hết quota
	// ngày, model không còn), để mỗi câu hỏi không tốn thêm một request hỏng.
	cooldown map[string]time.Time
}

// NewGeminiProvider tạo provider; model rỗng thì dùng model mặc định.
func NewGeminiProvider(apiKey, embedModel string, chatModels []string) *GeminiProvider {
	if embedModel == "" {
		embedModel = DefaultGeminiEmbedModel
	}
	if len(chatModels) == 0 {
		chatModels = DefaultGeminiChatModels
	}
	return &GeminiProvider{
		apiKey:     apiKey,
		embedModel: embedModel,
		chatModels: chatModels,
		baseURL:    geminiBaseURL,
		client:     &http.Client{},
		timeout:    geminiTimeout,
		maxRetries: geminiMaxRetries,
		backoff:    geminiBaseBackoff,
		log:        slog.Default(),
		now:        time.Now,
		cooldown:   map[string]time.Time{},
	}
}

type geminiPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiEmbedRequest struct {
	Model   string        `json:"model"`
	Content geminiContent `json:"content"`
	// Model mới (gemini-embedding-*) mặc định nhiều chiều hơn 768.
	OutputDimensionality int `json:"outputDimensionality"`
}

type geminiEmbedResponse struct {
	Embeddings []struct {
		Values []float32 `json:"values"`
	} `json:"embeddings"`
}

func (g *GeminiProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += geminiMaxEmbedBatch {
		batch := texts[start:min(start+geminiMaxEmbedBatch, len(texts))]
		v, err := g.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		vecs = append(vecs, v...)
	}
	return vecs, nil
}

func (g *GeminiProvider) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	model := "models/" + g.embedModel
	reqs := make([]geminiEmbedRequest, len(texts))
	for i, t := range texts {
		reqs[i] = geminiEmbedRequest{Model: model, Content: geminiContent{Parts: []geminiPart{{Text: t}}}, OutputDimensionality: Dimensions}
	}
	body, err := json.Marshal(map[string]any{"requests": reqs})
	if err != nil {
		return nil, fmt.Errorf("gemini embed: encode: %w", err)
	}

	resp, cancel, err := g.post(ctx, "/"+model+":batchEmbedContents", body, true)
	if err != nil {
		return nil, fmt.Errorf("gemini embed: %w", err)
	}
	defer cancel()
	defer resp.Body.Close()

	var res geminiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("gemini embed: decode: %w", err)
	}
	if len(res.Embeddings) != len(texts) {
		return nil, fmt.Errorf("gemini embed: nhận %d vector cho %d văn bản", len(res.Embeddings), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for i, e := range res.Embeddings {
		if len(e.Values) != Dimensions {
			return nil, fmt.Errorf("gemini embed: vector %d chiều, cần %d (model %s)", len(e.Values), Dimensions, g.embedModel)
		}
		vecs[i] = e.Values
	}
	return vecs, nil
}

type geminiStreamChunk struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error *geminiErrorBody `json:"error"`
}

// Các finishReason nghĩa là câu trả lời bị cắt vì chính sách, không phải xong.
var geminiBlockedReasons = map[string]bool{
	"SAFETY": true, "RECITATION": true, "BLOCKLIST": true, "PROHIBITED_CONTENT": true, "SPII": true,
}

// Stream gọi streamGenerateContent?alt=sse và đọc từng dòng "data: {...}".
// Chỉ retry hay đổi model trước khi có token nào được gửi (lỗi HTTP xảy ra
// trước body).
func (g *GeminiProvider) Stream(ctx context.Context, prompt string, out chan<- string) error {
	body, err := json.Marshal(map[string]any{
		"contents": []geminiContent{{Role: "user", Parts: []geminiPart{{Text: prompt}}}},
	})
	if err != nil {
		return fmt.Errorf("gemini stream: encode: %w", err)
	}

	resp, cancel, err := g.openStream(ctx, body)
	if err != nil {
		return fmt.Errorf("gemini stream: %w", err)
	}
	defer cancel()
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), geminiMaxSSELine)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue // dòng trống phân tách event, comment, event:/id:
		}
		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &chunk); err != nil {
			return fmt.Errorf("gemini stream: decode: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("gemini stream: %w", &APIError{StatusCode: chunk.Error.Code, Message: chunk.Error.Message})
		}
		if chunk.PromptFeedback != nil && chunk.PromptFeedback.BlockReason != "" {
			return fmt.Errorf("gemini stream: %w: prompt bị chặn (%s)", ErrBlocked, chunk.PromptFeedback.BlockReason)
		}
		for _, c := range chunk.Candidates {
			for _, p := range c.Content.Parts {
				if p.Thought || p.Text == "" {
					continue
				}
				if err := send(ctx, out, p.Text); err != nil {
					return err
				}
			}
			if geminiBlockedReasons[c.FinishReason] {
				return fmt.Errorf("gemini stream: %w: câu trả lời bị dừng (%s)", ErrBlocked, c.FinishReason)
			}
		}
	}
	if err := sc.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("gemini stream: đọc: %w", err)
	}
	return nil
}

// Thời gian bỏ qua một model sau khi nó hỏng: hết quota (khi server không
// nói chờ bao lâu) và model không còn (404).
const (
	geminiQuotaCooldown   = time.Minute
	geminiMissingCooldown = time.Hour
)

// openStream mở stream với model đầu tiên còn dùng được trong chatModels.
// Model hết quota (429), không còn (404) hay lỗi máy chủ (5xx) thì chuyển
// sang model kế tiếp; chỉ model cuối mới chờ quota theo phút, vì người dùng
// đang đợi câu trả lời.
func (g *GeminiProvider) openStream(ctx context.Context, body []byte) (*http.Response, context.CancelFunc, error) {
	models := g.availableChatModels()
	var lastErr error
	for i, model := range models {
		last := i == len(models)-1
		resp, cancel, err := g.post(ctx, "/models/"+model+":streamGenerateContent?alt=sse", body, last)
		if err == nil {
			return resp, cancel, nil
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			return nil, nil, err
		}
		switch {
		case apiErr.StatusCode == http.StatusTooManyRequests:
			g.coolDown(model, max(apiErr.RetryDelay, geminiQuotaCooldown))
		case apiErr.StatusCode == http.StatusNotFound:
			g.coolDown(model, geminiMissingCooldown)
		case apiErr.StatusCode >= 500:
			// post đã retry; thử model khác, không cần tạm bỏ qua model này.
		default:
			return nil, nil, err
		}
		lastErr = fmt.Errorf("model %s: %w", model, err)
		if !last {
			g.log.WarnContext(ctx, "gemini: falling back to next chat model", "model", model, "next", models[i+1], "err", err)
		}
	}
	return nil, nil, lastErr
}

// availableChatModels trả các model chưa bị tạm bỏ qua, giữ thứ tự. Nếu tất
// cả đều đang bị bỏ qua thì trả cả danh sách: thử lại còn hơn báo lỗi ngay.
func (g *GeminiProvider) availableChatModels() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	var out []string
	for _, m := range g.chatModels {
		if now.After(g.cooldown[m]) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return g.chatModels
	}
	return out
}

func (g *GeminiProvider) coolDown(model string, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cooldown[model] = g.now().Add(d)
}

type geminiErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Chỉ cần RetryInfo.retryDelay (dạng "56.9s"); các loại detail khác bỏ qua.
	Details []struct {
		RetryDelay string `json:"retryDelay"`
	} `json:"details"`
}

// post gửi body JSON tới baseURL+path, retry tối đa maxRetries lần với
// backoff luỹ thừa khi gặp 429, 5xx, lỗi mạng hoặc hết g.timeout trước khi
// có header. Thành công thì trả response đang mở
// cùng hàm huỷ timeout của lần gọi đó; người gọi đóng body rồi gọi cancel.
//
// waitOnQuota=false thì gặp 429 trả lỗi ngay thay vì chờ, để người gọi đổi
// sang model khác.
//
// API key đi qua header thay vì query string để không lộ trong log và lỗi.
func (g *GeminiProvider) post(ctx context.Context, path string, body []byte, waitOnQuota bool) (*http.Response, context.CancelFunc, error) {
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, g.timeout)
		req, err := http.NewRequestWithContext(actx, http.MethodPost, g.baseURL+path, bytes.NewReader(body))
		if err != nil {
			cancel()
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", g.apiKey)

		resp, err := g.client.Do(req)
		var lastErr error
		var status int
		var wait time.Duration
		switch {
		case err != nil:
			// Lỗi mạng hoặc quá g.timeout mà chưa có header: chưa đọc gì nên gọi
			// lại an toàn, trừ khi chính ctx của người gọi đã xong.
			cancel()
			if ctx.Err() != nil || attempt >= g.maxRetries {
				return nil, nil, err
			}
			lastErr, wait = err, g.backoffFor(attempt, "", 0)
		case resp.StatusCode == http.StatusOK:
			return resp, cancel, nil
		default:
			apiErr := readAPIError(resp)
			resp.Body.Close()
			cancel()
			quota := resp.StatusCode == http.StatusTooManyRequests
			retryable := (quota && waitOnQuota) || resp.StatusCode >= 500
			// Server đòi chờ quá lâu (quota theo ngày đòi hàng giờ): chờ cũng vô ích.
			if !retryable || attempt >= g.maxRetries || apiErr.RetryDelay > geminiMaxServerDelay {
				return nil, nil, apiErr
			}
			lastErr, status = apiErr, resp.StatusCode
			wait = g.backoffFor(attempt, resp.Header.Get("Retry-After"), apiErr.RetryDelay)
		}

		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait {
			return nil, nil, fmt.Errorf("%w: cần chờ %s mới gọi lại được (lỗi trước đó: %w)",
				context.DeadlineExceeded, wait.Round(time.Second), lastErr)
		}
		g.log.WarnContext(ctx, "gemini: retrying", "status", status, "err", lastErr, "attempt", attempt+1, "wait_ms", wait.Milliseconds())
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil, nil, fmt.Errorf("%w (lỗi trước đó: %w)", ctx.Err(), lastErr)
		}
	}
}

// backoffFor: backoff·2^attempt cộng jitter tới 50%, tối đa geminiMaxBackoff.
// Nếu server bảo chờ (header Retry-After tính bằng giây, hoặc RetryInfo
// trong body) lâu hơn thì chờ đúng mức đó, tối đa geminiMaxServerDelay.
func (g *GeminiProvider) backoffFor(attempt int, retryAfter string, serverDelay time.Duration) time.Duration {
	d := g.backoff << attempt
	d += time.Duration(rand.Int64N(int64(d)/2 + 1))
	d = min(d, geminiMaxBackoff)
	if s, err := strconv.Atoi(retryAfter); err == nil {
		serverDelay = max(serverDelay, time.Duration(s)*time.Second)
	}
	if serverDelay > d {
		d = min(serverDelay, geminiMaxServerDelay)
	}
	return d
}

func readAPIError(resp *http.Response) *APIError {
	// Lỗi quota kèm details khá dài; cắt ngắn thì mất RetryInfo.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body struct {
		Error geminiErrorBody `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	var delay time.Duration
	if json.Unmarshal(raw, &body) == nil {
		if body.Error.Message != "" {
			msg = body.Error.Message
		}
		for _, d := range body.Error.Details {
			if v, err := time.ParseDuration(d.RetryDelay); err == nil && v > 0 {
				delay = v
			}
		}
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg, RetryDelay: delay}
}
