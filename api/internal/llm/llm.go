// Package llm cài đặt service.LLMProvider: GeminiProvider gọi Gemini API,
// MockProvider chạy offline cho test và demo.
package llm

import "context"

// Dimensions là số chiều embedding, khớp cột chunks.embedding vector(768).
const Dimensions = 768

// send gửi s vào out, dừng nếu ctx bị huỷ trước khi người nhận kịp đọc.
func send(ctx context.Context, out chan<- string, s string) error {
	select {
	case out <- s:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
