// Package extract trích văn bản từ PDF, giữ số trang của từng đoạn.
package extract

import (
	"context"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// Page là văn bản đã chuẩn hoá của một trang. Number bắt đầu từ 1.
// Offset ký tự của chunk (char_start/char_end) tính trên Text này.
type Page struct {
	Number int
	Text   string
}

// PDF trả văn bản của mọi trang theo thứ tự, kể cả trang trống (Text rỗng),
// để len(pages) đúng bằng số trang của tài liệu.
func PDF(ctx context.Context, r io.ReaderAt, size int64) (pages []Page, err error) {
	// Thư viện pdf panic khi gặp file hỏng; đổi thành lỗi để không sập worker.
	defer func() {
		if rec := recover(); rec != nil {
			pages, err = nil, fmt.Errorf("PDF hỏng hoặc không đọc được: %v", rec)
		}
	}()

	rd, err := pdf.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("mở PDF: %w", err)
	}
	n := rd.NumPage()
	pages = make([]Page, 0, n)
	for i := 1; i <= n; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var text string
		if p := rd.Page(i); !p.V.IsNull() {
			text = pageText(p.Content().Text)
		}
		pages = append(pages, Page{Number: i, Text: text})
	}
	return pages, nil
}

// pageText dựng lại văn bản từ các glyph (theo thứ tự trong content stream)
// dựa vào toạ độ: đổi dòng khi Y đổi, cách đoạn ("\n\n") khi khoảng cách dòng
// lớn hoặc nhảy ngược lên (sang cột/khối khác), chèn dấu cách khi có khoảng
// trống ngang giữa hai glyph mà PDF không ghi ký tự cách.
func pageText(glyphs []pdf.Text) string {
	var b strings.Builder
	var prev pdf.Text
	have := false
	for _, g := range glyphs {
		s := cleanGlyph(g.S)
		if s == "" {
			continue
		}
		if have {
			fs := math.Max(math.Abs(prev.FontSize), 1)
			dy := g.Y - prev.Y
			switch {
			case dy < -1.7*fs || dy > fs:
				b.WriteString("\n\n")
			case math.Abs(dy) > 0.5*fs:
				b.WriteString("\n")
			default:
				gap := g.X - (prev.X + prev.W)
				if (gap > 0.2*fs || gap < -fs) && !endsWithSpace(&b) && !unicode.IsSpace([]rune(s)[0]) {
					b.WriteByte(' ')
				}
			}
		}
		b.WriteString(s)
		prev, have = g, true
	}
	return normalize(b.String())
}

// cleanGlyph bỏ ký tự điều khiển (Postgres text không nhận NUL) và glyph
// "\n" giả mà thư viện chèn sau mỗi toán tử TJ.
func cleanGlyph(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r), r == unicode.ReplacementChar:
			return -1
		}
		return r
	}, s)
}

func endsWithSpace(b *strings.Builder) bool {
	s := b.String()
	return s == "" || s[len(s)-1] == ' ' || s[len(s)-1] == '\n'
}

var manyNewlines = regexp.MustCompile(`\n{3,}`)

// normalize bỏ khoảng trắng cuối dòng, gộp nhiều dòng trống thành một.
func normalize(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	s = manyNewlines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(strings.ToValidUTF8(s, ""))
}
