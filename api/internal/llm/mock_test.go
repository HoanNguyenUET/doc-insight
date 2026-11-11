package llm

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func cosine(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

func TestMockEmbed(t *testing.T) {
	m := NewMockProvider()
	texts := []string{"Hợp đồng lao động có thời hạn 12 tháng.", "", "!!! ...", "Hợp đồng lao động có thời hạn 12 tháng."}
	vecs, err := m.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(vecs), len(texts))
	}
	for i, v := range vecs {
		if len(v) != Dimensions {
			t.Errorf("vec %d: %d dims, want %d", i, len(v), Dimensions)
		}
		if n := math.Sqrt(cosine(v, v)); math.Abs(n-1) > 1e-5 {
			t.Errorf("vec %d (%q): norm %v, want 1", i, texts[i], n)
		}
	}
	if c := cosine(vecs[0], vecs[3]); math.Abs(c-1) > 1e-6 {
		t.Errorf("same text: cosine %v, want 1", c)
	}
	if c := cosine(vecs[1], vecs[2]); c > 0.5 {
		t.Errorf("different texts without words should differ, cosine %v", c)
	}
}

func TestMockEmbedSharedWordsAreCloser(t *testing.T) {
	vecs, _ := NewMockProvider().Embed(context.Background(), []string{
		"thời hạn hợp đồng lao động là bao lâu",
		"Hợp đồng lao động có thời hạn 12 tháng kể từ ngày ký.",
		"Nhân viên được nghỉ phép 12 ngày mỗi năm.",
	})
	related, unrelated := cosine(vecs[0], vecs[1]), cosine(vecs[0], vecs[2])
	if related <= unrelated {
		t.Errorf("cosine(question, related)=%v <= cosine(question, unrelated)=%v", related, unrelated)
	}
}

func collect(t *testing.T, p *MockProvider, ctx context.Context, prompt string) ([]string, error) {
	t.Helper()
	out := make(chan string, 1024)
	err := p.Stream(ctx, prompt, out)
	close(out)
	var toks []string
	for tok := range out {
		toks = append(toks, tok)
	}
	return toks, err
}

func TestMockStreamAnswer(t *testing.T) {
	prompt := "Hướng dẫn, dạng [[n]] ở giữa dòng.\n\nĐoạn trích:\n\n" +
		"[[1]] (trang 3)\nNghỉ phép 12 ngày mỗi năm. Câu thứ hai bị bỏ.\n\n" +
		"[[2]] (trang 7)\nCâu hỏi: dòng này thuộc nội dung FAQ, không phải câu hỏi.\n\n" +
		"Câu hỏi: được nghỉ bao nhiêu ngày?\n"

	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{"cites each excerpt", prompt, "Theo tài liệu: Nghỉ phép 12 ngày mỗi năm. [[1]] Câu hỏi: dòng này thuộc nội dung FAQ, không phải câu hỏi. [[2]]"},
		{"no excerpts", "Đoạn trích:\n(không có đoạn nào)\n\nCâu hỏi: gì?\n", mockNoAnswer},
		{"long sentence capped", "[[1]] (trang 1)\n" + strings.Repeat("từ ", 100) + "\n", "Theo tài liệu: " + strings.TrimSpace(strings.Repeat("từ ", mockMaxSentenceWords)) + " [[1]]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, err := collect(t, &MockProvider{}, context.Background(), tt.prompt)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(toks, ""); got != tt.want {
				t.Errorf("answer =\n%q\nwant\n%q", got, tt.want)
			}
			if len(toks) != len(strings.Fields(tt.want)) {
				t.Errorf("got %d tokens, want one per word (%d)", len(toks), len(strings.Fields(tt.want)))
			}
		})
	}
}

func TestMockStreamDelayAndCancel(t *testing.T) {
	p := &MockProvider{Delay: 20 * time.Millisecond}
	prompt := "[[1]] (trang 1)\nmột hai ba bốn năm.\n" // 7 từ: "Theo tài liệu: một hai ba bốn năm. [[1]]"

	start := time.Now()
	if _, err := collect(t, p, context.Background(), prompt); err != nil {
		t.Fatal(err)
	}
	// 9 từ → 8 khoảng chờ.
	if d := time.Since(start); d < 8*p.Delay {
		t.Errorf("stream took %v, want >= %v", d, 8*p.Delay)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	toks, err := collect(t, p, ctx, prompt)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if len(toks) == 0 || len(toks) >= 9 {
		t.Errorf("got %d tokens before cancel, want a partial answer", len(toks))
	}
}
