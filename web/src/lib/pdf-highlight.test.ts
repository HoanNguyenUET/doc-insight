import { describe, expect, it } from "vitest";

import { locate, markItem } from "@/lib/pdf-highlight";

// Đổi kết quả locate sang dạng dễ so: chữ được tô của từng item.
function marked(items: string[], passage: string): string[] | null {
  const r = locate(items, passage);
  if (!r) return null;
  return [...r.entries()].sort(([a], [b]) => a - b).map(([i, [[s, e]]]) => items[i].slice(s, e));
}

describe("locate", () => {
  const items = ["Điều 12. Kiểm tra", " ", "chất lượng cọc khoan", "nhồi phải thí nghiệm", "Điều 13. Hồ sơ"];
  it.each([
    {
      name: "khoảng trắng và ngắt dòng khác nhau vẫn khớp",
      items,
      passage: "Kiểm tra chất lượng\ncọc khoan nhồi",
      want: ["Kiểm tra", "chất lượng cọc khoan", "nhồi"],
    },
    {
      name: "ligature của pdf.js (ﬁ) khớp với chữ thường",
      items: ["Certiﬁcate of", "completion"],
      passage: "Certificate of completion",
      want: ["Certiﬁcate of", "completion"],
    },
    {
      name: "dấu tiếng Việt dạng tổ hợp (NFD) khớp dạng dựng sẵn",
      items: ["Hồ sơ hoàn công".normalize("NFD")],
      passage: "Hồ sơ hoàn công",
      want: ["Hồ sơ hoàn công".normalize("NFD")],
    },
    {
      name: "bỏ rác Ω (U+2126) và ⊗ của bản trích cũ",
      items: ["Phan Boi Chau", "High School"],
      passage: "Phan Boi Chau\u2126 High \u2297School\u2126",
      want: ["Phan Boi Chau", "High School"],
    },
    {
      name: "giữa đoạn lệch thì khớp theo đầu và cuối",
      items: ["Mở đầu của đoạn nguồn dài", "phần giữa pdf.js đọc khác hẳn", "và đây là phần kết thúc đoạn"],
      passage: "Mở đầu của đoạn nguồn dài phần giữa bản Go đọc khác và đây là phần kết thúc đoạn",
      want: ["Mở đầu của đoạn nguồn dài", "phần giữa pdf.js đọc khác hẳn", "và đây là phần kết thúc đoạn"],
    },
    { name: "không có trên trang", items, passage: "Điều 99 không tồn tại ở đây", want: null },
    { name: "đoạn nguồn rỗng", items, passage: "  \n ", want: null },
  ])("$name", ({ items, passage, want }) => {
    expect(marked(items, passage)).toEqual(want);
  });
});

describe("markItem", () => {
  it.each([
    { name: "không tô", str: "a < b", ranges: undefined, want: "a &lt; b" },
    {
      name: "bọc phần được tô và escape HTML",
      str: 'x "<y>" & z',
      ranges: [[2, 7]] as [number, number][],
      want: 'x <mark class="c" data-cite="3">&quot;&lt;y&gt;&quot;</mark> &amp; z',
    },
  ])("$name", ({ str, ranges, want }) => {
    expect(markItem(str, ranges, "c", 3)).toBe(want);
  });
});
