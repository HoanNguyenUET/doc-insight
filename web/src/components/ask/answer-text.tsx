import { useMemo } from "react";

import { CitationChip } from "@/components/ask/citation-chip";
import type { Source } from "@/lib/api";
import { parseAnswer, splitOutside, type Part } from "@/lib/parse-answer";

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

  const [inDoc, outside] = splitOutside(paragraphs);
  // Đánh số liên tục để biết đoạn nào là đoạn cuối (hiện con trỏ khi stream).
  const renderParagraph = (parts: Part[], i: number) => (
    <p key={i} className="mb-[11px] whitespace-pre-wrap last:mb-0">
      {parts.map((p, j) => {
        if (p.kind === "text") return p.value;
        const src = byRef.get(p.n);
        // Số trích dẫn không có trong sources: model bịa, hiện nguyên văn.
        return src ? <CitationChip key={j} messageId={messageId} n={p.n} source={src} /> : `[${p.n}]`;
      })}
      {streaming && i === inDoc.length + outside.length - 1 && caret}
    </p>
  );
  if (outside.length === 0) return <>{inDoc.map(renderParagraph)}</>;

  return (
    <>
      {inDoc.map(renderParagraph)}
      <div className="mt-3 rounded-[8px] border border-dashed border-rule bg-field px-3.5 py-3 text-ink-2">
        <div className="mb-1.5 font-mono text-[10.5px] tracking-wide text-ink-3 uppercase">
          Ngoài tài liệu · kiến thức chung, không có nguồn
        </div>
        {outside.map((parts, k) => renderParagraph(parts, inDoc.length + k))}
      </div>
    </>
  );
}
