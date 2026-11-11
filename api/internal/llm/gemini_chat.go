package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Chat: stream câu trả lời và chuỗi dự phòng giữa các model.

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
