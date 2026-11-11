// Package pdftest dựng PDF tối giản cho test, không cần file mẫu.
//
// Mỗi tham số của Build là một trang. Mỗi "\n" là xuống dòng (Courier 10pt,
// leading 12pt); một dòng trống tạo khoảng cách đoạn. Chỉ hỗ trợ ASCII vì
// font chuẩn Courier dùng WinAnsiEncoding.
package pdftest

import (
	"bytes"
	"fmt"
	"strings"
)

func Build(pages ...string) []byte {
	return build(false, pages)
}

// BuildTeX giống Build nhưng bắt chước PDF do pdfTeX sinh: mỗi dòng in bằng
// toán tử TJ, font là bản subset (tên có tiền tố "ABCDEF+") và bảng mã đặt
// byte 10 là /Omega như mã OT1 của TeX.
func BuildTeX(pages ...string) []byte {
	return build(true, pages)
}

func build(tex bool, pages []string) []byte {
	var objs []string // objs[i] là thân object số i+1
	add := func(body string) int {
		objs = append(objs, body)
		return len(objs)
	}

	add("") // 1: Catalog, điền sau khi biết số object của Pages
	add("") // 2: Pages
	widths := strings.TrimSpace(strings.Repeat("600 ", 126-32+1))
	baseFont, encoding := "Courier", "/WinAnsiEncoding"
	if tex {
		baseFont = "ABCDEF+Courier"
		encoding = "<< /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [10 /Omega] >>"
	}
	font := add(fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding %s /FirstChar 32 /LastChar 126 /Widths [%s] >>", baseFont, encoding, widths))

	var kids []string
	for _, text := range pages {
		var s strings.Builder
		s.WriteString("BT /F1 10 Tf 12 TL 72 720 Td\n")
		for i, line := range strings.Split(text, "\n") {
			if i > 0 {
				s.WriteString("T*\n")
			}
			switch {
			case line == "":
			case tex:
				fmt.Fprintf(&s, "[(%s)] TJ\n", escape(line))
			default:
				fmt.Fprintf(&s, "(%s) Tj\n", escape(line))
			}
		}
		s.WriteString("ET")
		content := add(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", s.Len(), s.String()))
		page := add(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", font, content))
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
	}
	objs[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	objs[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return buf.Bytes()
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(s)
}
