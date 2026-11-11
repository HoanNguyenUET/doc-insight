// Tách câu trả lời của AI thành đoạn, chữ và trích dẫn [[n]].

export type Part = { kind: "text"; value: string } | { kind: "cite"; n: number };

const CITE = /\[\[(\d+)\]\]/g;
// Trích dẫn đang stream dở ở cuối chuỗi, vd. "[[" hoặc "[[2".
const PARTIAL_CITE = /\[\[\d*\]?$/;
// Nhãn mở đầu phần model trả lời bằng kiến thức chung (OutsideLabel trong
// api/internal/service/ask.go): phần này không có trong tài liệu.
export const OUTSIDE_LABEL = "Ngoài tài liệu:";

// parseAnswer chia câu trả lời thành đoạn (theo dòng trống), mỗi đoạn là chuỗi
// xen kẽ chữ và trích dẫn [[n]]. Đang stream thì bỏ trích dẫn viết dở ở cuối.
export function parseAnswer(text: string, streaming: boolean): Part[][] {
  const body = streaming ? text.replace(PARTIAL_CITE, "") : text;
  return body
    .split(/\n{2,}/)
    .map((para) => {
      const parts: Part[] = [];
      let last = 0;
      for (const m of para.matchAll(CITE)) {
        if (m.index > last) parts.push({ kind: "text", value: para.slice(last, m.index) });
        parts.push({ kind: "cite", n: Number(m[1]) });
        last = m.index + m[0].length;
      }
      if (last < para.length) parts.push({ kind: "text", value: para.slice(last) });
      return parts;
    })
    .filter((parts) => parts.length > 0);
}

// splitOutside tách các đoạn thành [từ tài liệu, ngoài tài liệu]: từ đoạn mở
// đầu bằng OUTSIDE_LABEL trở đi là kiến thức chung (nhãn được bỏ đi).
export function splitOutside(paragraphs: Part[][]): [Part[][], Part[][]] {
  const at = paragraphs.findIndex((p) => p[0]?.kind === "text" && p[0].value.trimStart().startsWith(OUTSIDE_LABEL));
  if (at < 0) return [paragraphs, []];
  const [first, ...rest] = paragraphs[at];
  const label = first as { kind: "text"; value: string };
  const stripped = label.value.trimStart().slice(OUTSIDE_LABEL.length).trimStart();
  const head: Part[] = stripped ? [{ kind: "text", value: stripped }, ...rest] : rest;
  return [paragraphs.slice(0, at), [head, ...paragraphs.slice(at + 1)].filter((p) => p.length > 0)];
}
