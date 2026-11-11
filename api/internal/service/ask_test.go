package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/internal/llm"
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
)

const askDocID = "00000000-0000-0000-0000-0000000000a1"

// fakeAskRepo xếp hạng chunk bằng cosine như pgvector, trên embedding do
// chính MockProvider tạo ra: test chạy được trọn luồng hỏi đáp mà không cần
// mạng hay database.
type fakeAskRepo struct {
	doc       repository.Document
	chunks    []repository.ChunkMatch
	vecs      [][]float32
	searchErr error
	gotLimit  int
}

func newFakeAskRepo(t *testing.T, status string, pages map[int]string) *fakeAskRepo {
	t.Helper()
	// Tài liệu được xử lý bằng chính mô hình của MockProvider.
	model := "mock"
	r := &fakeAskRepo{doc: repository.Document{ID: askDocID, Status: status, EmbedModel: &model}}
	nums := make([]int, 0, len(pages))
	for p := range pages {
		nums = append(nums, p)
	}
	sort.Ints(nums)
	for i, p := range nums {
		text := pages[p]
		r.chunks = append(r.chunks, repository.ChunkMatch{ID: int64(100 + i), Index: i, Content: text, PageNumber: p, CharStart: 10 * i, CharEnd: 10*i + len([]rune(text))})
	}
	texts := make([]string, len(r.chunks))
	for i, c := range r.chunks {
		texts[i] = c.Content
	}
	vecs, err := llm.NewMockProvider().Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	r.vecs = vecs
	return r
}

func (r *fakeAskRepo) GetDocument(_ context.Context, id string) (repository.Document, error) {
	if id != r.doc.ID {
		return repository.Document{}, repository.ErrNotFound
	}
	return r.doc, nil
}

func (r *fakeAskRepo) SearchChunks(_ context.Context, _ string, q []float32, limit int) ([]repository.ChunkMatch, error) {
	r.gotLimit = limit
	if r.searchErr != nil {
		return nil, r.searchErr
	}
	out := make([]repository.ChunkMatch, len(r.chunks))
	for i, c := range r.chunks {
		var dot float64
		for j := range q {
			dot += float64(q[j]) * float64(r.vecs[i][j])
		}
		c.Score = dot
		out[i] = c
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out[:min(limit, len(out))], nil
}

// recordingWriter ghi lại thứ tự sự kiện để kiểm tra sources đến trước token.
type recordingWriter struct {
	events    []string
	sources   []Source
	tokens    []string
	failAfter int // >0: WriteToken lỗi từ token thứ failAfter (client ngắt)
}

var errClientGone = errors.New("client đã ngắt")

func (w *recordingWriter) WriteSources(s []Source) error {
	w.events = append(w.events, "sources")
	w.sources = s
	return nil
}

func (w *recordingWriter) WriteToken(tok string) error {
	if w.failAfter > 0 && len(w.tokens)+1 >= w.failAfter {
		return errClientGone
	}
	w.events = append(w.events, "token")
	w.tokens = append(w.tokens, tok)
	return nil
}

func (w *recordingWriter) answer() string { return strings.Join(w.tokens, "") }

var handbook = map[int]string{
	1: "Giờ làm việc từ 8 giờ sáng đến 5 giờ chiều, nghỉ trưa một tiếng.",
	2: "Nhân viên chính thức được nghỉ phép 12 ngày mỗi năm. Phép chưa dùng được chuyển sang quý một năm sau.",
	3: "Lương được trả vào ngày 5 hằng tháng qua chuyển khoản.",
	4: "Trang phục công sở lịch sự, thứ sáu được mặc tự do.",
	5: "Thiết bị làm việc được cấp khi nhận việc và hoàn trả khi nghỉ việc.",
	6: "Bảo hiểm sức khoẻ áp dụng cho nhân viên và người thân.",
	7: "Đào tạo nội bộ diễn ra vào thứ tư hằng tuần.",
}

func newTestAskService(repo AskRepo) *AskService {
	return NewAskService(repo, &llm.MockProvider{})
}

func TestAskAnswersFromMostRelevantChunk(t *testing.T) {
	repo := newFakeAskRepo(t, repository.StatusReady, handbook)
	w := &recordingWriter{}

	if err := newTestAskService(repo).Ask(context.Background(), askDocID, "  Mỗi năm nhân viên được nghỉ phép bao nhiêu ngày?  ", w); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if repo.gotLimit != 5 {
		t.Errorf("search limit = %d, want 5", repo.gotLimit)
	}
	if len(w.events) < 2 || w.events[0] != "sources" || strings.Count(strings.Join(w.events, ","), "sources") != 1 {
		t.Fatalf("events = %v, want sources exactly once, before every token", w.events)
	}
	if len(w.sources) != 5 {
		t.Fatalf("got %d sources, want top 5 of %d chunks", len(w.sources), len(handbook))
	}
	for i, s := range w.sources {
		if s.N != i+1 {
			t.Errorf("source %d has N=%d, want %d", i, s.N, i+1)
		}
		if i > 0 && s.Score > w.sources[i-1].Score {
			t.Errorf("sources not sorted by score: %v", w.sources)
		}
	}
	top := w.sources[0]
	if top.Page != 2 || top.ChunkID != 101 || top.ChunkIndex != 1 || top.CharStart != 10 || top.CharEnd != 10+len([]rune(handbook[2])) || top.Content != handbook[2] {
		t.Errorf("top source = %+v, want the leave-policy chunk on page 2 with its offsets", top)
	}

	got := w.answer()
	if !strings.Contains(got, "Nhân viên chính thức được nghỉ phép 12 ngày mỗi năm. [[1]]") {
		t.Errorf("answer does not cite the top source as [[1]]:\n%s", got)
	}
	for _, s := range w.sources {
		if !strings.Contains(got, fmt.Sprintf("[[%d]]", s.N)) {
			t.Errorf("answer does not cite [[%d]]:\n%s", s.N, got)
		}
	}
	if strings.Contains(got, "[[6]]") {
		t.Errorf("answer cites a chunk that was not sent as a source:\n%s", got)
	}
}

func TestAskNoEmbeddedChunks(t *testing.T) {
	repo := newFakeAskRepo(t, repository.StatusReady, nil)
	w := &recordingWriter{}
	if err := newTestAskService(repo).Ask(context.Background(), askDocID, "Có gì trong tài liệu?", w); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if w.sources == nil || len(w.sources) != 0 {
		t.Errorf("sources = %#v, want empty non-nil slice (JSON [])", w.sources)
	}
	if !strings.Contains(w.answer(), "không có đoạn nào liên quan") {
		t.Errorf("answer = %q, want the no-information reply", w.answer())
	}
}

func TestAskRejects(t *testing.T) {
	ready := newFakeAskRepo(t, repository.StatusReady, handbook)
	dbDown := errors.New("db down")
	searchFails := newFakeAskRepo(t, repository.StatusReady, handbook)
	searchFails.searchErr = dbDown
	otherModel := newFakeAskRepo(t, repository.StatusReady, handbook)
	gemini := "gemini-embedding-001"
	otherModel.doc.EmbedModel = &gemini
	unknownModel := newFakeAskRepo(t, repository.StatusReady, handbook)
	unknownModel.doc.EmbedModel = nil

	tests := []struct {
		name     string
		repo     AskRepo
		docID    string
		question string
		want     error
	}{
		{"empty question", ready, askDocID, "   \n\t", ErrInvalidQuestion},
		{"question too long", ready, askDocID, strings.Repeat("á", maxQuestionRunes+1), ErrInvalidQuestion},
		{"unknown document", ready, "00000000-0000-0000-0000-0000000000ff", "Hỏi gì đó?", ErrDocNotFound},
		{"document still embedding", newFakeAskRepo(t, repository.StatusEmbedding, handbook), askDocID, "Hỏi gì đó?", ErrDocNotReady},
		{"document failed", newFakeAskRepo(t, repository.StatusFailed, handbook), askDocID, "Hỏi gì đó?", ErrDocNotReady},
		{"search error passes through", searchFails, askDocID, "Hỏi gì đó?", dbDown},
		// Vector của mô hình khác không so sánh được: tìm kiếm sẽ ra kết quả vô nghĩa.
		{"indexed with another model", otherModel, askDocID, "Hỏi gì đó?", ErrDocStale},
		{"indexed with unknown model", unknownModel, askDocID, "Hỏi gì đó?", ErrDocStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &recordingWriter{}
			err := newTestAskService(tt.repo).Ask(context.Background(), tt.docID, tt.question, w)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(w.events) != 0 {
				t.Errorf("writer got %v, want nothing written on error", w.events)
			}
		})
	}
}

func TestAskQuestionAtLimitIsAccepted(t *testing.T) {
	repo := newFakeAskRepo(t, repository.StatusReady, handbook)
	q := strings.Repeat("á", maxQuestionRunes)
	if err := newTestAskService(repo).Ask(context.Background(), askDocID, q, &recordingWriter{}); err != nil {
		t.Errorf("question of exactly %d runes: %v", maxQuestionRunes, err)
	}
}

// failingEmbedder bọc MockProvider nhưng Embed luôn lỗi.
type failingEmbedder struct{ llm.MockProvider }

func (*failingEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("hết quota")
}

func TestAskEmbedError(t *testing.T) {
	repo := newFakeAskRepo(t, repository.StatusReady, handbook)
	w := &recordingWriter{}
	err := NewAskService(repo, &failingEmbedder{}).Ask(context.Background(), askDocID, "Hỏi?", w)
	if err == nil || !strings.Contains(err.Error(), "hết quota") || len(w.events) != 0 {
		t.Errorf("err = %v, events = %v; want wrapped embed error and nothing written", err, w.events)
	}
}

func TestAskStopsWhenClientGone(t *testing.T) {
	repo := newFakeAskRepo(t, repository.StatusReady, handbook)
	w := &recordingWriter{failAfter: 3}
	svc := NewAskService(repo, &llm.MockProvider{Delay: 5 * time.Millisecond})

	start := time.Now()
	err := svc.Ask(context.Background(), askDocID, "Mỗi năm nhân viên được nghỉ phép bao nhiêu ngày?", w)
	if !errors.Is(err, errClientGone) {
		t.Fatalf("err = %v, want errClientGone", err)
	}
	if len(w.tokens) != 2 {
		t.Errorf("wrote %d tokens, want 2 before the failing write", len(w.tokens))
	}
	// Câu trả lời mẫu dài hàng chục từ; dừng sớm thì không phải chờ hết.
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("Ask took %v after the client left; stream was not cancelled", d)
	}
}

func TestAskContextCancelled(t *testing.T) {
	repo := newFakeAskRepo(t, repository.StatusReady, handbook)
	ctx, cancel := context.WithCancel(context.Background())
	w := &cancelOnFirstToken{cancel: cancel}
	err := NewAskService(repo, &llm.MockProvider{Delay: 5 * time.Millisecond}).Ask(ctx, askDocID, "nghỉ phép?", w)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

type cancelOnFirstToken struct {
	recordingWriter
	cancel context.CancelFunc
}

func (w *cancelOnFirstToken) WriteToken(tok string) error {
	w.cancel()
	return w.recordingWriter.WriteToken(tok)
}

func TestBuildPrompt(t *testing.T) {
	got := buildPrompt("Nghỉ phép bao nhiêu ngày?", []Source{
		{N: 1, Page: 2, Content: "  Nghỉ phép 12 ngày.\n"},
		{N: 2, Page: 9, Content: "Phép chuyển sang quý một."},
	})
	for _, want := range []string{
		"\n[[1]] (trang 2)\nNghỉ phép 12 ngày.\n",
		"\n[[2]] (trang 9)\nPhép chuyển sang quý một.\n",
		"\nCâu hỏi: Nghỉ phép bao nhiêu ngày?\n",
		// Frontend chỉ tách đoạn theo dòng trống, không render markdown.
		"không dùng markdown",
		// Được dùng kiến thức chung, nhưng tách riêng sau nhãn mà frontend
		// nhận ra, và không gắn trích dẫn vào phần đó.
		`"` + OutsideLabel + `"`,
		"không ghi trích dẫn",
		// Người dùng đối chiếu với bản gốc: "2 mm" không được thành "hai mi-li-mét".
		"giữ nguyên số liệu",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "[[1]] (trang") > strings.Index(got, "[[2]] (trang") {
		t.Error("excerpts out of order")
	}
	if strings.Contains(got, "không dùng kiến thức bên ngoài") {
		t.Error("prompt still forbids outside knowledge")
	}
	if !strings.Contains(buildPrompt("q", nil), "(không có đoạn nào)") {
		t.Error("empty-sources prompt should say there are no excerpts")
	}
}

// quotaErr giả lỗi hết lượt của provider (llm.APIError 429 có QuotaExhausted).
type quotaErr struct{}

func (quotaErr) Error() string        { return "gemini: HTTP 429: You exceeded your current quota" }
func (quotaErr) QuotaExhausted() bool { return true }

type quotaLLM struct {
	llm.MockProvider
	onEmbed bool
}

func (q *quotaLLM) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if q.onEmbed {
		return nil, fmt.Errorf("gemini embed: %w", quotaErr{})
	}
	return q.MockProvider.Embed(ctx, texts)
}

func (q *quotaLLM) Stream(context.Context, string, chan<- string) error {
	return fmt.Errorf("gemini stream: %w", quotaErr{})
}

func TestAskQuotaExhausted(t *testing.T) {
	tests := []struct {
		name    string
		onEmbed bool
	}{
		{"khi embed câu hỏi", true},
		{"khi sinh câu trả lời", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeAskRepo(t, repository.StatusReady, handbook)
			err := NewAskService(repo, &quotaLLM{onEmbed: tt.onEmbed}).Ask(context.Background(), askDocID, "Hỏi?", &recordingWriter{})
			if !errors.Is(err, ErrAIQuota) {
				t.Errorf("err = %v, want ErrAIQuota", err)
			}
		})
	}
	// Lỗi khác vẫn giữ nguyên, không bị gắn nhầm là hết lượt.
	repo := newFakeAskRepo(t, repository.StatusReady, handbook)
	if err := NewAskService(repo, &failingEmbedder{}).Ask(context.Background(), askDocID, "Hỏi?", &recordingWriter{}); errors.Is(err, ErrAIQuota) {
		t.Errorf("plain embed error marked as quota: %v", err)
	}
}
