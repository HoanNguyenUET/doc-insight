package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
)

var (
	ErrDocNotReady     = errors.New("tài liệu chưa xử lý xong")
	ErrInvalidQuestion = errors.New("câu hỏi không hợp lệ")
)

const (
	askTopK          = 5
	maxQuestionRunes = 1000
)

type AskRepo interface {
	GetDocument(ctx context.Context, id string) (repository.Document, error)
	SearchChunks(ctx context.Context, documentID string, embedding []float32, limit int) ([]repository.ChunkMatch, error)
}

// Source là một đoạn nguồn; câu trả lời trích dẫn nó bằng [[N]].
type Source struct {
	N         int     `json:"n"`
	ChunkID   int64   `json:"chunk_id"`
	Page      int     `json:"page"`
	CharStart int     `json:"char_start"`
	CharEnd   int     `json:"char_end"`
	Content   string  `json:"content"`
	Score     float64 `json:"score"`
}

// AnswerWriter nhận kết quả của Ask: WriteSources đúng một lần, trước mọi
// WriteToken (frontend cần nguồn sẵn để parse [[n]]). Lỗi từ writer (thường
// là client đã ngắt) làm Ask dừng stream và trả lỗi đó.
type AnswerWriter interface {
	WriteSources(sources []Source) error
	WriteToken(text string) error
}

type AskService struct {
	repo AskRepo
	llm  LLMProvider
	topK int
}

func NewAskService(repo AskRepo, llm LLMProvider) *AskService {
	return &AskService{repo: repo, llm: llm, topK: askTopK}
}

// Ask trả lời câu hỏi về một tài liệu: embed câu hỏi → top-K chunk gần nhất
// → dựng prompt có đánh số đoạn → stream câu trả lời vào w.
func (s *AskService) Ask(ctx context.Context, documentID, question string, w AnswerWriter) error {
	question = strings.TrimSpace(question)
	switch n := utf8.RuneCountInString(question); {
	case n == 0:
		return fmt.Errorf("%w: câu hỏi trống", ErrInvalidQuestion)
	case n > maxQuestionRunes:
		return fmt.Errorf("%w: dài quá %d ký tự", ErrInvalidQuestion, maxQuestionRunes)
	}

	doc, err := s.repo.GetDocument(ctx, documentID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrDocNotFound
	}
	if err != nil {
		return err
	}
	if doc.Status != repository.StatusReady {
		return fmt.Errorf("%w (trạng thái %s)", ErrDocNotReady, doc.Status)
	}
	log := slog.With("document_id", doc.ID)
	start := time.Now()

	vecs, err := s.llm.Embed(ctx, []string{question})
	if err != nil {
		return fmt.Errorf("embed câu hỏi: %w", err)
	}
	if len(vecs) != 1 || len(vecs[0]) == 0 {
		return fmt.Errorf("embed câu hỏi: nhận %d vector", len(vecs))
	}
	matches, err := s.repo.SearchChunks(ctx, doc.ID, vecs[0], s.topK)
	if err != nil {
		return err
	}

	sources := make([]Source, len(matches))
	for i, m := range matches {
		sources[i] = Source{N: i + 1, ChunkID: m.ID, Page: m.PageNumber, CharStart: m.CharStart, CharEnd: m.CharEnd, Content: m.Content, Score: m.Score}
	}
	if err := w.WriteSources(sources); err != nil {
		return fmt.Errorf("gửi nguồn: %w", err)
	}

	tokens, err := s.stream(ctx, buildPrompt(question, sources), w)
	if err != nil {
		return err
	}
	log.InfoContext(ctx, "ask: answered", "sources", len(sources), "tokens", tokens, "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// stream chạy LLM trong goroutine sống trong phạm vi request: nó dừng khi
// Stream trả về, và Stream dừng khi streamCtx bị huỷ. Channel luôn được đọc
// cạn nên Stream không bao giờ kẹt ở lệnh gửi.
func (s *AskService) stream(ctx context.Context, prompt string, w AnswerWriter) (int, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make(chan string, 32)
	errc := make(chan error, 1)
	go func() {
		errc <- s.llm.Stream(streamCtx, prompt, out)
		close(out)
	}()

	var tokens int
	var writeErr error
	for tok := range out {
		if writeErr != nil {
			continue
		}
		if err := w.WriteToken(tok); err != nil {
			writeErr = fmt.Errorf("gửi token: %w", err)
			cancel()
			continue
		}
		tokens++
	}
	streamErr := <-errc
	if writeErr != nil {
		return tokens, writeErr
	}
	if streamErr != nil {
		return tokens, fmt.Errorf("stream câu trả lời: %w", streamErr)
	}
	return tokens, nil
}

// OutsideLabel mở đầu phần trả lời bằng kiến thức chung (không có trong
// tài liệu, không có trích dẫn). Frontend dựa vào nhãn này để hiện phần đó
// tách riêng; đổi ở đây thì đổi cả web/src/components/ask/answer-text.tsx.
const OutsideLabel = "Ngoài tài liệu:"

// buildPrompt đánh số đoạn trích [[1]]..[[n]] theo đúng thứ tự sources.
// MockProvider đọc lại định dạng này: dòng "[[n]] (trang p)" mở đầu mỗi
// đoạn, dòng "Câu hỏi:" mở đầu phần câu hỏi.
func buildPrompt(question string, sources []Source) string {
	var b strings.Builder
	b.WriteString(`Bạn là trợ lý trả lời câu hỏi về một tài liệu nội bộ. Ưu tiên thông tin trong các đoạn trích bên dưới, và không bịa ra điều gì về tài liệu.
Sau mỗi ý lấy từ đoạn trích, ghi trích dẫn dạng [[n]], với n là số của đoạn trích chứa thông tin đó; một ý dựa trên nhiều đoạn thì ghi [[1]][[3]].
Nếu cần kiến thức chung (giải thích khái niệm, bối cảnh, gợi ý, soạn văn bản) hoặc câu hỏi nằm ngoài tài liệu, bạn được trả lời bằng hiểu biết của mình, nhưng đặt toàn bộ phần đó ở cuối, mở đầu bằng đúng "` + OutsideLabel + `" ở đầu một đoạn mới, và không ghi trích dẫn trong phần đó. Nếu tài liệu không đề cập điều được hỏi, nói rõ điều đó trước.
Trả lời bằng tiếng Việt, ngắn gọn.
Viết văn xuôi, không dùng markdown (không **, *, #, gạch đầu dòng, bảng); muốn tách ý thì xuống đoạn bằng một dòng trống.

Đoạn trích:
`)
	if len(sources) == 0 {
		b.WriteString("(không có đoạn nào)\n")
	}
	for _, src := range sources {
		b.WriteString("\n[[" + strconv.Itoa(src.N) + "]] (trang " + strconv.Itoa(src.Page) + ")\n")
		b.WriteString(strings.TrimSpace(src.Content) + "\n")
	}
	b.WriteString("\nCâu hỏi: " + question + "\n")
	return b.String()
}
