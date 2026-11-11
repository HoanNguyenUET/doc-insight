"use client";

import { useMemo } from "react";

import { CitationChip } from "@/components/ask/citation-chip";
import type { Source } from "@/lib/api";

type Part = { kind: "text"; value: string } | { kind: "cite"; n: number };

const CITE = /\[\[(\d+)\]\]/g;
// Trích dẫn đang stream dở ở cuối chuỗi, vd. "[[" hoặc "[[2".
const PARTIAL_CITE = /\[\[\d*\]?$/;

// parseAnswer chia câu trả lời thành đoạn (theo dòng trống), mỗi đoạn là chuỗi
// xen kẽ chữ và trích dẫn [[n]].
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

  return (
    <>
      {paragraphs.map((parts, i) => (
        <p key={i} className="mb-[11px] whitespace-pre-wrap last:mb-0">
          {parts.map((p, j) => {
            if (p.kind === "text") return p.value;
            const src = byRef.get(p.n);
            // Số trích dẫn không có trong sources: model bịa, hiện nguyên văn.
            return src ? <CitationChip key={j} messageId={messageId} n={p.n} source={src} /> : `[${p.n}]`;
          })}
          {streaming && i === paragraphs.length - 1 && caret}
        </p>
      ))}
    </>
  );
}
