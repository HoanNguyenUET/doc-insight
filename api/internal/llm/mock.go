package llm

import (
	"context"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// MockProvider chạy hoàn toàn offline, kết quả tất định.
//
// Embed băm FNV từng từ (feature hashing) vào vector 768 chiều rồi chuẩn hoá
// độ dài 1: cùng văn bản ra cùng vector, và văn bản có chung từ thì gần nhau,
// nên tìm kiếm tương đồng vẫn trả đoạn liên quan khi demo không có mạng.
//
// Stream ghép câu đầu của mỗi đoạn trích trong prompt thành câu trả lời mẫu
// kèm trích dẫn [[n]], phát từng từ cách nhau Delay.
type MockProvider struct {
	Delay time.Duration
}

func NewMockProvider() *MockProvider {
	return &MockProvider{Delay: 30 * time.Millisecond}
}

func (m *MockProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(texts))
	for i, t := range texts {
		vecs[i] = mockVector(t)
	}
	return vecs, nil
}

func mockVector(text string) []float32 {
	acc := make([]float64, Dimensions)
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		h := fnvHash(w)
		sign := 1.0
		if h>>63 == 1 {
			sign = -1
		}
		acc[h%Dimensions] += sign
	}

	var norm float64
	for _, x := range acc {
		norm += x * x
	}
	// Không có từ nào (hoặc các từ triệt tiêu nhau): lấy vector giả ngẫu nhiên
	// gieo từ hash của cả văn bản, để không bao giờ trả vector 0 (cosine NaN).
	if norm == 0 {
		h := fnvHash(text)
		r := rand.New(rand.NewPCG(h, h>>1|1))
		for i := range acc {
			acc[i] = r.NormFloat64()
			norm += acc[i] * acc[i]
		}
	}
	norm = math.Sqrt(norm)

	v := make([]float32, Dimensions)
	for i, x := range acc {
		v[i] = float32(x / norm)
	}
	return v
}

func fnvHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// excerptHeader khớp dòng mở đầu đoạn trích "[[n]] (trang p)" mà service
// dựng trong prompt; "Câu hỏi:" mở đầu phần câu hỏi ở cuối prompt.
var (
	excerptHeader  = regexp.MustCompile(`(?m)^\[\[(\d+)\]\][^\n]*\n`)
	questionMarker = regexp.MustCompile(`(?m)^Câu hỏi:`)
)

const (
	mockMaxSentenceWords = 40
	mockNoAnswer         = "Tài liệu không có đoạn nào liên quan đến câu hỏi này."
)

func (m *MockProvider) Stream(ctx context.Context, prompt string, out chan<- string) error {
	words := strings.Fields(mockAnswer(prompt))
	for i, w := range words {
		if i > 0 {
			w = " " + w
			if m.Delay > 0 {
				t := time.NewTimer(m.Delay)
				select {
				case <-t.C:
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				}
			}
		}
		if err := send(ctx, out, w); err != nil {
			return err
		}
	}
	return nil
}

func mockAnswer(prompt string) string {
	// Lấy dòng "Câu hỏi:" cuối cùng: nội dung đoạn trích (vd. tài liệu FAQ)
	// cũng có thể có dòng bắt đầu như vậy.
	if locs := questionMarker.FindAllStringIndex(prompt, -1); len(locs) > 0 {
		prompt = prompt[:locs[len(locs)-1][0]]
	}
	headers := excerptHeader.FindAllStringSubmatchIndex(prompt, -1)
	if len(headers) == 0 {
		return mockNoAnswer
	}

	var b strings.Builder
	b.WriteString("Theo tài liệu:")
	for i, h := range headers {
		end := len(prompt)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		s := firstSentence(prompt[h[1]:end])
		if s == "" {
			continue
		}
		n := prompt[h[2]:h[3]]
		b.WriteString(" " + s + " [[" + n + "]]")
	}
	return b.String()
}

// firstSentence lấy câu đầu (tới . ! ? đầu tiên), tối đa mockMaxSentenceWords từ.
func firstSentence(text string) string {
	words := strings.Fields(text)
	for i, w := range words {
		if strings.ContainsAny(w[len(w)-1:], ".!?") || i+1 == mockMaxSentenceWords {
			words = words[:i+1]
			break
		}
	}
	return strings.Join(words, " ")
}
