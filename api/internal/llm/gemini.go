package llm

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

// QuotaExhausted cho service nhận ra lỗi hết lượt gọi mà không phụ thuộc
// package này (service kiểm tra qua interface).
func (e *APIError) QuotaExhausted() bool { return e.StatusCode == http.StatusTooManyRequests }

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
