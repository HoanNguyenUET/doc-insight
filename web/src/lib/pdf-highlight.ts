// Tìm đoạn nguồn trong text layer của pdf.js để tô sáng trên trang PDF thật.
//
// Offset của chunk tính trên văn bản do API trích (Go), còn text layer là
// văn bản pdf.js tự đọc: cùng nội dung nhưng khác cách ngắt dòng, khoảng
// trắng, ligature, thứ tự vài khối. Vì vậy so khớp trên chuỗi đã chuẩn hoá
// (NFKC, chữ thường, chỉ giữ chữ và số) rồi ánh xạ ngược về từng item.

// Phạm vi [start, end) theo code unit trong item.str của pdf.js.
export type ItemRanges = Map<number, [number, number][]>;

// Độ dài mỏ neo khi không khớp nguyên đoạn: khớp phần đầu và phần cuối.
const ANCHOR = 24;

type Flat = { text: string; item: number[]; start: number[]; end: number[] };

// flatten nối các item thành một chuỗi đã chuẩn hoá, nhớ mỗi ký tự đến từ
// item nào, ở code unit nào.
function flatten(items: string[]): Flat {
  const out: Flat = { text: "", item: [], start: [], end: [] };
  items.forEach((str, idx) => {
    let i = 0;
    for (const ch of str) {
      for (const n of normalize(ch)) {
        out.text += n;
        out.item.push(idx);
        out.start.push(i);
        out.end.push(i + ch.length);
      }
      i += ch.length;
    }
  });
  return out;
}

// Ký hiệu Ohm (U+2126) là rác do bản trích cũ sinh ra từ font TeX, không
// có trong text layer. Phải bỏ trước NFKC: NFKC đổi nó thành chữ Omega.
function normalize(s: string): string {
  return s
    .replace(/Ω/g, "")
    .normalize("NFKC")
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]/gu, "");
}

// locate trả các phạm vi cần tô trong từng item, hoặc null nếu không tìm
// thấy đoạn nguồn trên trang.
export function locate(items: string[], passage: string): ItemRanges | null {
  const hay = flatten(items);
  const needle = normalize(passage);
  if (!needle || !hay.text) return null;

  let from = hay.text.indexOf(needle);
  let to = from + needle.length;
  if (from < 0) {
    const head = needle.slice(0, ANCHOR);
    const tail = needle.slice(-ANCHOR);
    from = hay.text.indexOf(head);
    if (from < 0) return null;
    const t = hay.text.indexOf(tail, from + head.length);
    // Đuôi ở quá xa thì coi như không khớp, chỉ tô đúng độ dài đoạn nguồn.
    to = t >= 0 && t + tail.length - from <= needle.length * 1.5 ? t + tail.length : from + needle.length;
    to = Math.min(to, hay.text.length);
  }

  // Mỗi item tô một khoảng liền, từ ký tự khớp đầu tới ký tự khớp cuối (phủ
  // luôn khoảng trắng ở giữa).
  const ranges: ItemRanges = new Map();
  for (let k = from; k < to; k++) {
    const r = ranges.get(hay.item[k])?.[0];
    if (r) r[1] = hay.end[k];
    else ranges.set(hay.item[k], [[hay.start[k], hay.end[k]]]);
  }
  return ranges;
}

const ESCAPES: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" };
const escapeHtml = (s: string) => s.replace(/[&<>"]/g, (c) => ESCAPES[c]);

// markItem dựng HTML cho một item của text layer, bọc phần cần tô trong
// <mark data-cite="nonce">. react-pdf lọc HTML này trước khi gắn vào trang.
export function markItem(str: string, ranges: [number, number][] | undefined, className: string, nonce: number): string {
  if (!ranges) return escapeHtml(str);
  let html = "";
  let pos = 0;
  for (const [a, b] of ranges) {
    html += escapeHtml(str.slice(pos, a)) + `<mark class="${className}" data-cite="${nonce}">${escapeHtml(str.slice(a, b))}</mark>`;
    pos = b;
  }
  return html + escapeHtml(str.slice(pos));
}
