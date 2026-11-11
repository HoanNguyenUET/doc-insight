"use client";

import { useEffect, useRef, useState } from "react";

import { QuoteAskIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import type { Document } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { displayName } from "@/lib/format";
import { usePage } from "@/lib/queries";

const MIN_PICK_CHARS = 12;
// Câu hỏi gửi lên tối đa 1000 ký tự; chừa chỗ cho phần người dùng gõ.
const MAX_QUOTE_CHARS = 600;

type Pick = { top: number; left: number; text: string };

// Cột trái: văn bản từng trang, tô sáng đoạn nguồn của trích dẫn đang chọn,
// và nút "Hỏi về đoạn này" khi bôi đen.
export function DocumentReader({ doc }: { doc: Document }) {
  const currentPage = useAskStore((s) => s.currentPage);
  const setCurrentPage = useAskStore((s) => s.setCurrentPage);
  const activeCitation = useAskStore((s) => s.activeCitation);
  const setPendingQuote = useAskStore((s) => s.setPendingQuote);
  const { data: pageData, isError, error, isFetching } = usePage(doc.id, currentPage, true);

  const scrollRef = useRef<HTMLDivElement>(null);
  const sheetRef = useRef<HTMLElement>(null);
  const markRef = useRef<HTMLElement>(null);
  const [pick, setPick] = useState<Pick | null>(null);

  const pageCount = doc.page_count ?? pageData?.page_count ?? 1;
  // keepPreviousData giữ trang cũ khi đang tải trang mới: chỉ tô sáng khi
  // dữ liệu đang hiển thị đúng là trang của trích dẫn.
  const cited =
    activeCitation && pageData && pageData.page_number === activeCitation.page ? activeCitation : null;

  // Đổi trang xong mới cuộn: phần tử đoạn nguồn chỉ có trong DOM sau khi
  // pageData của trang mới về. Vì vậy effect phụ thuộc cả hai.
  useEffect(() => {
    if (!activeCitation || !pageData || pageData.page_number !== activeCitation.page) return;
    markRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [activeCitation, pageData]);

  // Lật trang bằng tay thì về đầu trang và bỏ nút trích dẫn.
  const shownPage = pageData?.page_number;
  useEffect(() => {
    setPick(null);
    if (!useAskStore.getState().activeCitation) scrollRef.current?.scrollTo({ top: 0 });
  }, [shownPage]);

  const turn = (delta: number) => {
    const next = Math.min(pageCount, Math.max(1, currentPage + delta));
    if (next !== currentPage) setCurrentPage(next);
  };

  const onMouseUp = () => {
    const sel = window.getSelection();
    const text = sel?.toString().trim() ?? "";
    const scroll = scrollRef.current;
    if (!sel || sel.rangeCount === 0 || text.length <= MIN_PICK_CHARS || !scroll || !sheetRef.current?.contains(sel.anchorNode)) {
      setPick(null);
      return;
    }
    const r = sel.getRangeAt(0).getBoundingClientRect();
    const box = scroll.getBoundingClientRect();
    setPick({ top: r.top - box.top + scroll.scrollTop, left: r.left - box.left + r.width / 2, text });
  };

  const quote = () => {
    if (!pick) return;
    const text = pick.text.replace(/\s+/g, " ");
    setPendingQuote(text.length > MAX_QUOTE_CHARS ? text.slice(0, MAX_QUOTE_CHARS).trimEnd() + "…" : text);
    setPick(null);
    window.getSelection()?.removeAllRanges();
  };

  return (
    <section className="flex min-w-0 flex-[1_1_56%] flex-col border-r border-rule bg-field">
      <div className="flex h-[50px] flex-none items-center justify-between gap-3.5 border-b border-rule bg-paper px-4">
        <div className="truncate text-sm font-medium">{doc.filename}</div>
        <div className="flex items-center gap-px rounded-[7px] bg-field p-0.5">
          <PagerButton onClick={() => turn(-1)} disabled={currentPage <= 1} label="Trang trước">
            ‹
          </PagerButton>
          <span className="px-2 font-mono text-xs text-ink-2">
            <b>{currentPage}</b> / {pageCount}
          </span>
          <PagerButton onClick={() => turn(1)} disabled={currentPage >= pageCount} label="Trang sau">
            ›
          </PagerButton>
        </div>
      </div>

      <div
        ref={scrollRef}
        onMouseUp={onMouseUp}
        className="relative flex flex-1 justify-center overflow-y-auto px-5 pt-7 pb-10"
      >
        <article
          ref={sheetRef}
          className="relative min-h-[800px] w-full max-w-[620px] bg-white px-14 pt-[52px] pb-20 shadow-[0_1px_2px_rgba(26,28,31,.06),0_8px_26px_rgba(26,28,31,.07)]"
        >
          <div className="mb-[26px] flex justify-between gap-4 border-b border-rule-soft pb-[13px] font-mono text-[10.5px] text-ink-3">
            <span className="truncate">{displayName(doc.filename)}</span>
            <span className="whitespace-nowrap">
              Trang <b>{pageData?.page_number ?? currentPage}</b>
            </span>
          </div>

          <div
            className={`font-serif text-[15.5px] leading-[1.72] whitespace-pre-wrap text-[#24262A] transition-opacity ${isFetching && pageData?.page_number !== currentPage ? "opacity-50" : ""}`}
          >
            {isError ? (
              <p className="py-16 text-center font-sans text-sm text-ink-3">{error.message}</p>
            ) : !pageData ? (
              <p className="py-16 text-center font-sans text-sm text-ink-3">Đang tải trang…</p>
            ) : pageData.text.trim() === "" ? (
              <p className="py-16 text-center font-sans text-sm text-ink-3">Trang này không có văn bản.</p>
            ) : (
              <PageText text={pageData.text} cited={cited} markRef={markRef} />
            )}
          </div>

          <div className="absolute right-14 bottom-[26px] left-14 flex justify-between border-t border-rule-soft pt-3 font-mono text-[10px] text-ink-3">
            <span>Lưu hành nội bộ</span>
            <span>
              {pageData?.page_number ?? currentPage} / {pageCount}
            </span>
          </div>
        </article>

        {pick && (
          <Button
            size="sm"
            className="absolute z-40 h-auto -translate-x-1/2 -translate-y-[130%] gap-[7px] rounded-[7px] bg-ink px-3 py-1.5 text-[13px] font-[450] text-white shadow-[0_4px_14px_rgba(0,0,0,.2)] hover:bg-ink/90"
            style={{ top: pick.top, left: pick.left }}
            // Giữ vùng chọn khi bấm nút.
            onMouseDown={(e) => e.preventDefault()}
            onMouseUp={(e) => e.stopPropagation()}
            onClick={quote}
          >
            <QuoteAskIcon className="size-3.5 stroke-[1.8]" />
            Hỏi về đoạn này
          </Button>
        )}
      </div>
    </section>
  );
}

function PagerButton({
  children,
  label,
  ...props
}: { children: React.ReactNode; label: string } & React.ComponentProps<"button">) {
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={label}
      className="h-6 w-[26px] rounded-[5px] text-ink-2 hover:bg-paper disabled:opacity-30"
      {...props}
    >
      {children}
    </Button>
  );
}

// PageText tách văn bản trang thành [trước, đoạn nguồn, sau]. Offset của
// chunk tính theo rune (code point), nên cắt bằng Array.from chứ không dùng
// slice trên UTF-16.
function PageText({
  text,
  cited,
  markRef,
}: {
  text: string;
  cited: { charStart: number; charEnd: number; nonce: number } | null;
  markRef: React.RefObject<HTMLElement | null>;
}) {
  if (!cited) return <>{text}</>;
  const runes = Array.from(text);
  return (
    <>
      {runes.slice(0, cited.charStart).join("")}
      <mark
        // key theo nonce: bấm lại cùng trích dẫn thì animation chạy lại.
        key={cited.nonce}
        ref={markRef}
        className="animate-flash rounded-[2px] bg-marker-soft py-0.5 text-inherit shadow-[0_0_0_5px_var(--color-marker-soft)]"
      >
        {runes.slice(cited.charStart, cited.charEnd).join("")}
      </mark>
      {runes.slice(cited.charEnd).join("")}
    </>
  );
}
