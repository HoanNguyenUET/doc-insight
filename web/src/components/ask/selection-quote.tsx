import { useEffect, useState } from "react";

import { QuoteAskIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { useAskStore } from "@/lib/ask-store";

const MIN_PICK_CHARS = 12;
// Câu hỏi gửi lên tối đa 1000 ký tự; chừa chỗ cho phần người dùng gõ.
const MAX_QUOTE_CHARS = 600;

type Pick = { page: number; top: number; left: number; text: string };

// SelectionQuote: bôi đen chữ trên trang thì hiện nút "Hỏi về đoạn này" ngay
// trên vùng chọn; bấm thì đưa đoạn đó vào ô nhập câu hỏi. Đặt bên trong
// khung cuộn (scrollRef) để nút trôi theo trang.
export function SelectionQuote({
  scrollRef,
  sheetRef,
  page,
}: {
  scrollRef: React.RefObject<HTMLElement | null>;
  sheetRef: React.RefObject<HTMLElement | null>;
  page: number;
}) {
  const setPendingQuote = useAskStore((s) => s.setPendingQuote);
  const [pick, setPick] = useState<Pick | null>(null);

  useEffect(() => {
    const scroll = scrollRef.current;
    if (!scroll) return;
    const onMouseUp = () => {
      const sel = window.getSelection();
      const text = sel?.toString().trim() ?? "";
      if (
        !sel ||
        sel.rangeCount === 0 ||
        text.length <= MIN_PICK_CHARS ||
        !sheetRef.current?.contains(sel.anchorNode)
      ) {
        setPick(null);
        return;
      }
      const r = sel.getRangeAt(0).getBoundingClientRect();
      const box = scroll.getBoundingClientRect();
      setPick({ page, top: r.top - box.top + scroll.scrollTop, left: r.left - box.left + r.width / 2, text });
    };
    scroll.addEventListener("mouseup", onMouseUp);
    return () => scroll.removeEventListener("mouseup", onMouseUp);
  }, [scrollRef, sheetRef, page]);

  // Nút gắn với trang nơi bôi chọn nên tự ẩn khi sang trang khác.
  if (!pick || pick.page !== page) return null;

  const quote = () => {
    const text = pick.text.replace(/\s+/g, " ");
    setPendingQuote(text.length > MAX_QUOTE_CHARS ? text.slice(0, MAX_QUOTE_CHARS).trimEnd() + "…" : text);
    setPick(null);
    window.getSelection()?.removeAllRanges();
  };

  return (
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
  );
}
