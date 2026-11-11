package chunker

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

func TestEstimateTokens(t *testing.T) {
	tests := map[string]int{
		"":                        0,
		"ab":                      0,
		"abc":                     1,
		"Tiếng Việt":              3, // 10 rune, không phải 14 byte
		strings.Repeat("ố", 1500): 500,
	}
	for s, want := range tests {
		if got := EstimateTokens(s); got != want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", s, got, want)
		}
	}
}

func spanTexts(r []rune, spans []span) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = string(r[s.start:s.end])
	}
	return out
}

func TestSentences(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"basic", "Một. Hai! Ba? Bốn", []string{"Một.", "Hai!", "Ba?", "Bốn"}},
		{"decimal and version are not boundaries", "Số pi là 3.14 và bản v1.2.3 ổn. Hết.", []string{"Số pi là 3.14 và bản v1.2.3 ổn.", "Hết."}},
		{"closing quote and paren stay with sentence", `Anh nói "xong rồi." (Thật vậy.) Tiếp`, []string{`Anh nói "xong rồi."`, `(Thật vậy.)`, "Tiếp"}},
		{"ellipsis and repeated marks", "Chờ đã… Thật sao?! Ừ.", []string{"Chờ đã…", "Thật sao?!", "Ừ."}},
		{"single newline is a line wrap", "Câu này bị\nngắt dòng. Câu sau.", []string{"Câu này bị\nngắt dòng.", "Câu sau."}},
		{"blank line ends a heading", "Chương 1\n\nNội dung chương", []string{"Chương 1", "Nội dung chương"}},
		{"blank line with spaces", "Tiêu đề  \n  \n  Thân", []string{"Tiêu đề", "Thân"}},
		{"surrounding whitespace", "  \n Một.  \n\n ", []string{"Một."}},
		{"empty", " \n\t ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := []rune(tt.in)
			got := spanTexts(r, sentences(r))
			if fmt.Sprint(got) != fmt.Sprint(tt.want) || len(got) != len(tt.want) {
				t.Errorf("sentences = %q, want %q", got, tt.want)
			}
		})
	}
}

// checkInvariants kiểm các tính chất phải đúng với mọi đầu vào.
func checkInvariants(t *testing.T, text string, chunks []Chunk, opt Options) {
	t.Helper()
	r := []rune(text)
	maxRunes := opt.TargetTokens * runesPerToken
	covered := make([]bool, len(r))
	for i, c := range chunks {
		if c.Page != 7 {
			t.Fatalf("chunk %d page = %d, want 7", i, c.Page)
		}
		if c.Start < 0 || c.End > len(r) || c.End <= c.Start {
			t.Fatalf("chunk %d bad range [%d,%d) for %d runes", i, c.Start, c.End, len(r))
		}
		if c.Content != string(r[c.Start:c.End]) {
			t.Fatalf("chunk %d content does not match its offsets", i)
		}
		if c.Tokens != EstimateTokens(c.Content) || c.Tokens > opt.TargetTokens || c.End-c.Start > maxRunes {
			t.Fatalf("chunk %d has %d runes / %d tokens, limit %d tokens", i, c.End-c.Start, c.Tokens, opt.TargetTokens)
		}
		if unicode.IsSpace(r[c.Start]) || unicode.IsSpace(r[c.End-1]) {
			t.Fatalf("chunk %d not trimmed: %q", i, c.Content)
		}
		if i > 0 {
			prev := chunks[i-1]
			if c.Start <= prev.Start || c.End <= prev.End {
				t.Fatalf("chunk %d [%d,%d) does not advance past chunk %d [%d,%d)", i, c.Start, c.End, i-1, prev.Start, prev.End)
			}
			if overlap := prev.End - c.Start; overlap > opt.OverlapTokens*runesPerToken {
				t.Fatalf("chunk %d overlaps previous by %d runes, max %d", i, overlap, opt.OverlapTokens*runesPerToken)
			}
		}
		for p := c.Start; p < c.End; p++ {
			covered[p] = true
		}
	}
	for p, ok := range covered {
		if !ok && !unicode.IsSpace(r[p]) {
			t.Fatalf("rune %d (%q) not in any chunk", p, r[p])
		}
	}
}

var words = strings.Fields("tài liệu hệ thống dữ liệu người dùng truy vấn kết quả mô hình ngôn ngữ trích dẫn trang đoạn văn bản the quick brown fox jumps over lazy dog 2024 v1.2 3.14")

func randomText(rng *rand.Rand, sentences int) string {
	var b strings.Builder
	for s := 0; s < sentences; s++ {
		n := 1 + rng.Intn(40)
		if rng.Intn(25) == 0 {
			n = 300 + rng.Intn(500) // câu rất dài, buộc phải cắt giữa câu
		}
		for w := 0; w < n; w++ {
			if w > 0 {
				b.WriteString([]string{" ", " ", " ", "\n"}[rng.Intn(4)])
			}
			b.WriteString(words[rng.Intn(len(words))])
		}
		b.WriteString([]string{". ", "! ", "? ", ".\n\n", "\n\n", "… "}[rng.Intn(6)])
		if rng.Intn(60) == 0 {
			b.WriteString(strings.Repeat("x", 2000+rng.Intn(2000)) + " ") // "từ" không có khoảng trắng
		}
	}
	return b.String()
}

func TestSplitPageInvariantsRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	opts := []Options{DefaultOptions, {TargetTokens: 40, OverlapTokens: 10}, {TargetTokens: 100, OverlapTokens: 0}, {TargetTokens: 1, OverlapTokens: 1}}
	for i := 0; i < 300; i++ {
		text := randomText(rng, rng.Intn(80))
		opt := opts[i%len(opts)]
		checkInvariants(t, text, SplitPage(7, text, opt), opt)
	}
}

func makeSentences(n, words int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Câu %d%s.", i, strings.Repeat(" chữ", words))
	}
	return out
}

func TestSplitPageCutsAtSentenceBoundaries(t *testing.T) {
	// Câu ~60 rune (< overlap 150): mọi chunk phải bắt đầu và kết thúc đúng ranh giới câu.
	sents := makeSentences(200, 13)
	text := strings.Join(sents, " ")
	chunks := SplitPage(7, text, DefaultOptions)
	checkInvariants(t, text, chunks, DefaultOptions)
	if len(chunks) < 5 {
		t.Fatalf("got %d chunks, want several", len(chunks))
	}

	isSentence := map[string]bool{}
	for _, s := range sents {
		isSentence[s] = true
	}
	for i, c := range chunks {
		parts := strings.SplitAfter(c.Content, ". ")
		for _, p := range parts {
			if !isSentence[strings.TrimSpace(p)] {
				t.Fatalf("chunk %d contains a partial sentence %q", i, p)
			}
		}
		if c.Tokens < DefaultOptions.TargetTokens-30 && i < len(chunks)-1 {
			t.Errorf("chunk %d only %d tokens; packing should fill close to %d", i, c.Tokens, DefaultOptions.TargetTokens)
		}
		if i > 0 {
			overlap := chunks[i-1].End - c.Start
			if overlap <= 0 {
				t.Errorf("chunk %d does not overlap previous", i)
			}
		}
	}
}

func TestSplitPageOverlapFallsBackToWordBoundary(t *testing.T) {
	// Câu ~400 rune > overlap: vùng overlap không có đầu câu nên lùi về đầu một từ.
	text := strings.Join(makeSentences(20, 100), " ")
	chunks := SplitPage(7, text, DefaultOptions)
	checkInvariants(t, text, chunks, DefaultOptions)
	r := []rune(text)
	for i := 1; i < len(chunks); i++ {
		c := chunks[i]
		if overlap := chunks[i-1].End - c.Start; overlap <= 0 {
			t.Errorf("chunk %d has no overlap", i)
		}
		if c.Start > 0 && !unicode.IsSpace(r[c.Start-1]) {
			t.Errorf("chunk %d starts mid-word: %q", i, string(r[c.Start-1:c.Start+5]))
		}
	}
}

func TestSplitPageSmallAndEmpty(t *testing.T) {
	if got := SplitPage(1, "  \n\n ", DefaultOptions); got != nil {
		t.Errorf("blank page: got %v, want nil", got)
	}
	got := SplitPage(3, "  Một câu ngắn. Hai câu.  ", DefaultOptions)
	want := Chunk{Page: 3, Start: 2, End: 24, Content: "Một câu ngắn. Hai câu.", Tokens: 7}
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %+v, want [%+v]", got, want)
	}
}

func TestSplitPageWithoutOverlap(t *testing.T) {
	opt := Options{TargetTokens: 100, OverlapTokens: 0}
	text := strings.Join(makeSentences(50, 10), " ")
	chunks := SplitPage(7, text, opt)
	checkInvariants(t, text, chunks, opt)
	for i := 1; i < len(chunks); i++ {
		if chunks[i].Start < chunks[i-1].End {
			t.Errorf("chunk %d overlaps although OverlapTokens=0", i)
		}
	}
}
