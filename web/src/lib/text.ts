// foldVietnamese chuẩn hoá để so khớp không phân biệt dấu và hoa thường:
// "Điều 208" -> "dieu 208". NFKD tách dấu (kể cả văn bản pdf.js trả về dạng tổ
// hợp) và tách ligature ("ﬁ" -> "fi").
export function foldVietnamese(s: string): string {
  return s.normalize("NFKD").replace(/\p{M}/gu, "").replace(/[đĐ]/g, "d").toLowerCase();
}
