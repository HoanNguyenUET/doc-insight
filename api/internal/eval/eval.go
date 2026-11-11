// Package eval chấm chất lượng câu trả lời của hệ thống hỏi đáp trên một bộ
// câu hỏi có đáp án (cases.json): truy xuất có lấy đúng đoạn nguồn không,
// trích dẫn có hợp lệ và trỏ đúng nguồn không, câu trả lời có chứa thông tin
// cần có không, và với câu hỏi ngoài tài liệu thì có nói rõ là không đề cập.
//
// Các hàm chấm điểm ở đây thuần (không I/O); client.go gọi API thật.
package eval

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

// Case là một câu hỏi trong bộ đánh giá.
type Case struct {
	ID       string `json:"id"`
	Doc      string `json:"doc"` // tên file trong thư mục docs của bộ đánh giá
	Question string `json:"question"`
	Expect   Expect `json:"expect"`
}

// Expect mô tả câu trả lời đúng. Trường nào rỗng thì bỏ qua tiêu chí đó.
type Expect struct {
	// SourceText: đoạn nguồn đúng phải chứa chuỗi này (không phân biệt hoa
	// thường, dấu).
	SourceText string `json:"source_text,omitempty"`
	// Pages: đoạn nguồn đúng phải nằm ở một trong các trang này.
	Pages []int `json:"pages,omitempty"`
	// AnswerAny: mỗi nhóm phải có ít nhất một cụm xuất hiện trong phần trả
	// lời từ tài liệu, vd. [["ba bộ", "3 bộ"], ["bảy ngày", "7 ngày"]].
	AnswerAny [][]string `json:"answer_any,omitempty"`
	// NotInDoc: tài liệu không có thông tin này; phần từ tài liệu phải nói rõ
	// là không đề cập và không gắn trích dẫn.
	NotInDoc bool `json:"not_in_doc,omitempty"`
}

// Source là một nguồn API trả về (event sources).
type Source struct {
	Chunk int `json:"chunk"`
	Page  int `json:"page"`
}

// Run là kết quả thô của một lần hỏi.
type Run struct {
	Sources []Source
	// Chunks: nội dung đầy đủ của từng đoạn trong tài liệu, theo chỉ số đoạn.
	Chunks  map[int]string
	Answer  string
	Err     string
	Latency time.Duration
}

// Check là kết quả một tiêu chí.
type Check struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Note string `json:"note,omitempty"`
}

// Result là điểm của một câu hỏi.
type Result struct {
	ID        string  `json:"id"`
	Question  string  `json:"question"`
	Pass      bool    `json:"pass"`
	Checks    []Check `json:"checks"`
	HitRank   int     `json:"hit_rank"` // vị trí (từ 1) của nguồn đúng đầu tiên; 0 = không có
	LatencyMS int64   `json:"latency_ms"`
	Answer    string  `json:"answer"`
	Err       string  `json:"error,omitempty"`
}

var (
	citeRe = regexp.MustCompile(`\[\[(\d+)\]\]`)
	// Những cách model nói tài liệu không có thông tin.
	abstainPhrases = []string{"không đề cập", "không có thông tin", "không nêu", "không nói", "không được đề cập"}
)

// Score chấm một câu hỏi.
func Score(c Case, r Run) Result {
	res := Result{ID: c.ID, Question: c.Question, Answer: r.Answer, Err: r.Err, LatencyMS: r.Latency.Milliseconds()}
	if r.Err != "" {
		res.Checks = []Check{{Name: "không lỗi", OK: false, Note: r.Err}}
		return res
	}

	// Phần sau nhãn là kiến thức chung, không tính vào câu trả lời từ tài liệu.
	inDoc, _, _ := strings.Cut(r.Answer, service.OutsideLabel)
	cites := citations(inDoc)
	add := func(name string, ok bool, note string) { res.Checks = append(res.Checks, Check{name, ok, note}) }

	valid := true
	for _, n := range citations(r.Answer) {
		if n < 1 || n > len(r.Sources) {
			valid = false
		}
	}
	add("trích dẫn hợp lệ", valid, "")

	if c.Expect.NotInDoc {
		add("nói rõ không đề cập", containsAny(inDoc, abstainPhrases), "")
		add("không trích dẫn bừa", len(cites) == 0, "")
	} else {
		if c.Expect.SourceText != "" || len(c.Expect.Pages) > 0 {
			res.HitRank = hitRank(c.Expect, r)
			add("truy xuất đúng nguồn", res.HitRank > 0, "")
			cited := false
			for _, n := range cites {
				if n >= 1 && n <= len(r.Sources) && isHit(c.Expect, r.Sources[n-1], r.Chunks) {
					cited = true
				}
			}
			add("trích dẫn trỏ đúng nguồn", cited, "")
		}
		for _, group := range c.Expect.AnswerAny {
			add("có \""+strings.Join(group, "\" hoặc \"")+"\"", containsAny(inDoc, group), "")
		}
	}

	res.Pass = true
	for _, ch := range res.Checks {
		res.Pass = res.Pass && ch.OK
	}
	return res
}

func hitRank(e Expect, r Run) int {
	for i, s := range r.Sources {
		if isHit(e, s, r.Chunks) {
			return i + 1
		}
	}
	return 0
}

func isHit(e Expect, s Source, chunks map[int]string) bool {
	if len(e.Pages) > 0 && !containsInt(e.Pages, s.Page) {
		return false
	}
	return e.SourceText == "" || strings.Contains(Fold(chunks[s.Chunk]), Fold(e.SourceText))
}

func citations(s string) []int {
	var out []int
	for _, m := range citeRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}

func containsAny(s string, phrases []string) bool {
	f := Fold(s)
	for _, p := range phrases {
		if strings.Contains(f, Fold(p)) {
			return true
		}
	}
	return false
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Fold chuẩn hoá để so khớp: bỏ dấu, chữ thường, gộp khoảng trắng.
func Fold(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'đ' || r == 'Đ':
			r = 'd'
		case unicode.IsSpace(r):
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// Summary là điểm tổng của cả bộ.
type Summary struct {
	Cases        int     `json:"cases"`
	Passed       int     `json:"passed"`
	PassRate     float64 `json:"pass_rate"`
	RetrievalHit float64 `json:"retrieval_hit_rate"` // tỉ lệ câu có nguồn đúng trong top-K
	MRR          float64 `json:"mrr"`                // trung bình 1/vị trí nguồn đúng
	AvgLatencyMS int64   `json:"avg_latency_ms"`
}

// Summarize tính điểm tổng; các tỉ lệ truy xuất chỉ tính trên câu có kỳ vọng
// về nguồn.
func Summarize(cases []Case, results []Result) Summary {
	s := Summary{Cases: len(results)}
	var retrieval int
	var hits, rr float64
	var latency int64
	for i, r := range results {
		if r.Pass {
			s.Passed++
		}
		latency += r.LatencyMS
		e := cases[i].Expect
		if !e.NotInDoc && (e.SourceText != "" || len(e.Pages) > 0) {
			retrieval++
			if r.HitRank > 0 {
				hits++
				rr += 1 / float64(r.HitRank)
			}
		}
	}
	if s.Cases > 0 {
		s.PassRate = float64(s.Passed) / float64(s.Cases)
		s.AvgLatencyMS = latency / int64(s.Cases)
	}
	if retrieval > 0 {
		s.RetrievalHit = hits / float64(retrieval)
		s.MRR = rr / float64(retrieval)
	}
	return s
}
