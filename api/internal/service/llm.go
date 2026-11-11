package service

import "context"

// LLMProvider là mô hình ngôn ngữ mà service dùng. Cài đặt nằm ở
// internal/llm: GeminiProvider (API thật) và MockProvider (offline).
type LLMProvider interface {
	// Embed trả đúng một vector cho mỗi văn bản, cùng thứ tự.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Stream gửi từng mảnh câu trả lời vào out cho tới khi xong, lỗi hoặc ctx
	// bị huỷ. Stream không đóng out: người gọi sở hữu channel.
	Stream(ctx context.Context, prompt string, out chan<- string) error
}
