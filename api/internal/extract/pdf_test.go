package extract

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/HoanNguyenUET/doc-insight/api/internal/pdftest"
	"github.com/ledongthuc/pdf"
)

func extractBytes(t *testing.T, data []byte) ([]Page, error) {
	t.Helper()
	return PDF(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func TestPDFKeepsPageNumbers(t *testing.T) {
	data := pdftest.Build(
		"Trang mot. Cau thu hai (co ngoac).",
		"",
		"Dong mot\nDong hai\n\nDoan moi.",
	)
	pages, err := extractBytes(t, data)
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	want := []Page{
		{1, "Trang mot. Cau thu hai (co ngoac)."},
		{2, ""},
		{3, "Dong mot\nDong hai\n\nDoan moi."},
	}
	if len(pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %#v", len(pages), len(want), pages)
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Errorf("page %d = %#v, want %#v", i+1, pages[i], want[i])
		}
	}
}

// Thư viện pdf chèn glyph "\n" giả sau mỗi TJ và giải mã nó qua bảng mã của
// font. Với font TeX, byte 10 là Ω (OT1) hay ⊗ (OMS), nên ký tự rác này hiện
// giữa văn bản.
func TestPDFDropsFakeNewlineGlyphs(t *testing.T) {
	pages, err := extractBytes(t, pdftest.BuildTeX("Nguyen Thuc Hoan\nEmail: a@b.c\n\nTechnical Skills"))
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	want := "Nguyen Thuc Hoan\nEmail: a@b.c\n\nTechnical Skills"
	if len(pages) != 1 || pages[0].Text != want {
		t.Errorf("text = %q, want %q", pages[0].Text, want)
	}
}

func TestPDFRejectsGarbage(t *testing.T) {
	for name, data := range map[string][]byte{
		"not a pdf": []byte("hello world"),
		"truncated": pdftest.Build("abc")[:200],
		"empty":     nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := extractBytes(t, data); err == nil {
				t.Error("want error, got nil")
			}
		})
	}
}

func TestPDFStopsOnCanceledContext(t *testing.T) {
	data := pdftest.Build("a", "b")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PDF(ctx, bytes.NewReader(data), int64(len(data))); err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// glyph tạo một ký tự 10pt rộng 6pt tại (x, y).
func glyph(s string, x, y float64) pdf.Text {
	return pdf.Text{S: s, X: x, Y: y, W: 6, FontSize: 10}
}

func word(s string, x, y float64) []pdf.Text {
	var out []pdf.Text
	for i, r := range []rune(s) {
		out = append(out, glyph(string(r), x+float64(i)*6, y))
	}
	return out
}

func TestPageTextLayout(t *testing.T) {
	tests := []struct {
		name   string
		glyphs []pdf.Text
		want   string
	}{
		{
			// TJ [(Xin) -400 (chao)]: không có ký tự cách, chỉ có khoảng trống ngang.
			name:   "gap becomes space",
			glyphs: append(word("Xin", 0, 700), word("chao", 30, 700)...),
			want:   "Xin chao",
		},
		{
			name:   "kerning is not a space",
			glyphs: append(word("Xin", 0, 700), word("chao", 18.5, 700)...),
			want:   "Xinchao",
		},
		{
			name:   "no double space",
			glyphs: append(word("a ", 0, 700), word("b", 40, 700)...),
			want:   "a b",
		},
		{
			name:   "next line",
			glyphs: append(word("mot", 0, 700), word("hai", 0, 688)...),
			want:   "mot\nhai",
		},
		{
			name:   "paragraph gap",
			glyphs: append(word("mot", 0, 700), word("hai", 0, 670)...),
			want:   "mot\n\nhai",
		},
		{
			name:   "jump up to next column",
			glyphs: append(word("cot1", 0, 100), word("cot2", 300, 700)...),
			want:   "cot1\n\ncot2",
		},
		{
			name:   "drops control chars and TJ newline pseudo glyph",
			glyphs: []pdf.Text{glyph("a", 0, 700), glyph("\x00", 6, 700), glyph("b", 6, 700), glyph("\n", 99, 0), glyph("c", 12, 700)},
			want:   "abc",
		},
		{
			name:   "vietnamese",
			glyphs: append(word("Tiếng", 0, 700), word("Việt.", 40, 700)...),
			want:   "Tiếng Việt.",
		},
		{name: "empty", glyphs: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pageText(tt.glyphs, nil); got != tt.want {
				t.Errorf("pageText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	got := normalize("  a  \nb\n\n\n\n c \n\n")
	if want := "a\nb\n\n c"; got != want {
		t.Errorf("normalize = %q, want %q", got, want)
	}
	if strings.ContainsRune(normalize("x\xffy"), '�') {
		t.Error("invalid UTF-8 not removed")
	}
}
