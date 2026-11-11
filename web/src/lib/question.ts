// Giới hạn khớp với API: service.maxQuestionRunes (api/internal/service/ask.go)
// và service.MaxUploadBytes (api/internal/service/document.go).
export const MAX_QUESTION_CHARS = 1000;
export const MAX_UPLOAD_BYTES = 50 << 20;
// Đoạn trích bôi đen tối đa, để còn chỗ cho phần người dùng gõ.
export const MAX_QUOTE_CHARS = 600;

const wrap = (quote: string, text: string) => `Về đoạn trích «${quote}»: ${text}`;

// buildQuestion dựng câu hỏi gửi lên API. Đoạn trích nằm trong câu hỏi để
// truy xuất tìm đúng vùng đó.
export function buildQuestion(text: string, quote: string | null): string {
  return quote ? wrap(quote, text) : text;
}

// questionBudget là số ký tự người dùng còn được gõ khi đã có đoạn trích.
export function questionBudget(quote: string | null): number {
  return MAX_QUESTION_CHARS - (quote ? wrap(quote, "").length : 0);
}
