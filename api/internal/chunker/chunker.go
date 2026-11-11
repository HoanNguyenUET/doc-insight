// Package chunker cắt văn bản của một trang thành các đoạn (chunk) có độ dài
// mục tiêu, chồng lấn nhau, ưu tiên cắt ở ranh giới câu.
//
// Chunk không vượt qua ranh giới trang: char_start/char_end là offset trong
// văn bản của đúng một trang (để frontend tô sáng đoạn nguồn). Offset tính
// theo rune (ký tự Unicode), nửa mở [Start, End), và Content luôn bằng
// []rune(pageText)[Start:End].
package chunker

import (
	"unicode"
	"unicode/utf8"
)

type Options struct {
	// TargetTokens là độ dài tối đa của một chunk, tính bằng EstimateTokens.
	TargetTokens int
	// OverlapTokens là phần cuối chunk trước được lặp lại ở đầu chunk sau.
	OverlapTokens int
}

var DefaultOptions = Options{TargetTokens: 500, OverlapTokens: 50}

// runesPerToken khớp với EstimateTokens.
const runesPerToken = 3

type Chunk struct {
	Page    int
	Start   int
	End     int
	Content string
	Tokens  int
}

// EstimateTokens ước lượng số token: len([]rune(s))/3. Đủ dùng để giới hạn
// kích thước chunk; không dùng để tính tiền API.
func EstimateTokens(s string) int {
	return utf8.RuneCountInString(s) / runesPerToken
}

type span struct{ start, end int }

// SplitPage cắt văn bản của trang page. Trang trống trả về nil.
func SplitPage(page int, text string, opt Options) []Chunk {
	maxRunes := max(opt.TargetTokens*runesPerToken, 1)
	overlapRunes := min(max(opt.OverlapTokens*runesPerToken, 0), maxRunes-1)

	r := []rune(text)
	var units []span
	for _, s := range sentences(r) {
		units = append(units, splitLong(r, s, maxRunes)...)
	}
	if len(units) == 0 {
		return nil
	}

	spans := pack(r, units, maxRunes, overlapRunes)
	chunks := make([]Chunk, len(spans))
	for i, s := range spans {
		content := string(r[s.start:s.end])
		chunks[i] = Chunk{Page: page, Start: s.start, End: s.end, Content: content, Tokens: EstimateTokens(content)}
	}
	return chunks
}

// pack gom các câu liên tiếp vào chunk tới khi chạm maxRunes. Chunk sau bắt
// đầu ở đầu câu sớm nhất nằm trong vùng overlap của chunk trước; nếu vùng đó
// không có đầu câu nào (câu cuối dài hơn overlap) thì lùi về đầu một từ.
func pack(r []rune, units []span, maxRunes, overlapRunes int) []span {
	var out []span
	start, idx := units[0].start, 0
	for {
		j, end := idx, start
		for j < len(units) && units[j].end-start <= maxRunes {
			end = units[j].end
			j++
		}
		out = append(out, span{start, end})
		if j == len(units) {
			return out
		}

		next := units[j].start
		// Chặn dưới: không vượt overlap, chunk sau phải chứa trọn câu j
		// (để luôn tiến lên), và không quay về điểm bắt đầu cũ.
		lb := max(end-overlapRunes, units[j].end-maxRunes, start+1)
		if overlapRunes > 0 {
			if p, ok := overlapStart(r, units[idx:j], lb, end); ok {
				next = p
			}
		}
		start = next
		for units[idx].end <= start {
			idx++
		}
	}
}

func overlapStart(r []rune, units []span, lb, end int) (int, bool) {
	for _, u := range units {
		if u.start >= lb && u.start < end {
			return u.start, true
		}
	}
	for p := max(lb, 1); p < end; p++ {
		if !unicode.IsSpace(r[p]) && unicode.IsSpace(r[p-1]) {
			return p, true
		}
	}
	return 0, false
}

// sentences tách r thành các câu, bỏ khoảng trắng hai đầu. Câu kết thúc sau
// dấu . ! ? … (kèm ngoặc/nháy đóng) khi theo sau là khoảng trắng hoặc hết
// văn bản, hoặc tại dòng trống (ranh giới đoạn). Xuống dòng đơn chỉ là ngắt
// dòng trong PDF nên không tính là hết câu.
func sentences(r []rune) []span {
	var out []span
	n := len(r)
	i := 0
	for {
		for i < n && unicode.IsSpace(r[i]) {
			i++
		}
		if i >= n {
			return out
		}
		start, end := i, n
	scan:
		for j := i; j < n; j++ {
			switch {
			case isTerminator(r[j]):
				k := j + 1
				for k < n && (isTerminator(r[k]) || isCloser(r[k])) {
					k++
				}
				if k == n || unicode.IsSpace(r[k]) {
					end = k
					break scan
				}
				j = k - 1
			case r[j] == '\n' && blankLineAfter(r, j):
				end = j
				break scan
			}
		}
		for end > start && unicode.IsSpace(r[end-1]) {
			end--
		}
		out = append(out, span{start, end})
		i = end
	}
}

func isTerminator(c rune) bool {
	switch c {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

func isCloser(c rune) bool {
	switch c {
	case '"', '\'', ')', ']', '}', '”', '’', '»':
		return true
	}
	return false
}

// blankLineAfter: sau r[j] == '\n' là một dòng chỉ có khoảng trắng.
func blankLineAfter(r []rune, j int) bool {
	for k := j + 1; k < len(r); k++ {
		switch r[k] {
		case '\n':
			return true
		case ' ', '\t', '\r':
			continue
		}
		return false
	}
	return false
}

// splitLong chia một câu dài hơn maxRunes thành các mẩu ≤ maxRunes, cắt ở
// khoảng trắng cuối cùng trong cửa sổ; không có khoảng trắng thì cắt cứng.
func splitLong(r []rune, s span, maxRunes int) []span {
	var out []span
	for s.end-s.start > maxRunes {
		cut := s.start + maxRunes
		for c := cut; c > s.start; c-- {
			if unicode.IsSpace(r[c]) {
				cut = c
				break
			}
		}
		end := cut
		for end > s.start && unicode.IsSpace(r[end-1]) {
			end--
		}
		out = append(out, span{s.start, end})
		s.start = cut
		for s.start < s.end && unicode.IsSpace(r[s.start]) {
			s.start++
		}
	}
	if s.end > s.start {
		out = append(out, s)
	}
	return out
}
