import { useMemo } from "react";

import { CitationChip } from "@/components/ask/citation-chip";
import type { Source } from "@/lib/api";

type Part = { kind: "text"; value: string } | { kind: "cite"; n: number };

const CITE = /\[\[(\d+)\]\]/g;
// Nhãn mở đầu phần model trả lời bằng kiến thức chung (OutsideLabel trong
// api/internal/service/ask.go): phần này không có trong tài liệu.
const OUTSIDE_LABEL = "Ngoài tài liệu:";
// Trích dẫn đang stream dở ở cuối chuỗi, vd. "[[" hoặc "[[2".
const PARTIAL_CITE = /\[\[\d*\]?$/;

// parseAnswer chia câu trả lời thành đoạn (theo dòng trống), mỗi đoạn là chuỗi
// xen kẽ chữ và trích dẫn [[n]].
function parseAnswer(text: string, streaming: boolean): Part[][] {
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

export function AnswerText({
  messageId,
  text,
  sources,
  streaming = false,
}: {
  messageId: string;
  text: string;
  sources: Source[];
  streaming?: boolean;
}) {
  const paragraphs = useMemo(() => parseAnswer(text, streaming), [text, streaming]);
  const byRef = useMemo(() => new Map(sources.map((s) => [s.ref, s])), [sources]);
  const caret = (
    <span className="ml-0.5 inline-block h-[15px] w-0.5 animate-caret bg-blueprint align-[-2px]" aria-hidden />
  );

  if (paragraphs.length === 0) return streaming ? <p>{caret}</p> : null;

  // Từ đoạn mở đầu bằng nhãn trở đi là kiến thức chung, hiện trong khung riêng.
  const split = paragraphs.findIndex((parts) => parts[0]?.kind === "text" && parts[0].value.trimStart().startsWith(OUTSIDE_LABEL));
  const renderParagraph = (parts: Part[], i: number) => (
    <p key={i} className="mb-[11px] whitespace-pre-wrap last:mb-0">
      {parts.map((p, j) => {
        if (p.kind === "text") return p.value;
        const src = byRef.get(p.n);
        // Số trích dẫn không có trong sources: model bịa, hiện nguyên văn.
        return src ? <CitationChip key={j} messageId={messageId} n={p.n} source={src} /> : `[${p.n}]`;
      })}
      {streaming && i === paragraphs.length - 1 && caret}
    </p>
  );
  if (split < 0) return <>{paragraphs.map(renderParagraph)}</>;

  const outside = paragraphs.slice(split).map((parts, k) => (k === 0 ? dropLabel(parts) : parts));
  return (
    <>
      {paragraphs.slice(0, split).map(renderParagraph)}
      <div className="mt-3 rounded-[8px] border border-dashed border-rule bg-field px-3.5 py-3 text-ink-2">
        <div className="mb-1.5 font-mono text-[10.5px] tracking-wide text-ink-3 uppercase">
          Ngoài tài liệu · kiến thức chung, không có nguồn
        </div>
        {outside.map((parts, k) => renderParagraph(parts, split + k))}
      </div>
    </>
  );
}

// dropLabel bỏ nhãn "Ngoài tài liệu:" ở đầu đoạn (khung đã có tiêu đề riêng).
function dropLabel([first, ...rest]: Part[]): Part[] {
  if (first?.kind !== "text") return [first, ...rest];
  return [{ kind: "text", value: first.value.trimStart().slice(OUTSIDE_LABEL.length).trimStart() }, ...rest];
}
