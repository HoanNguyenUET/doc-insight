import { Badge } from "@/components/ui/badge";
import type { Source } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { cn } from "@/lib/utils";

// Thẻ trích dẫn [[n]]: bấm thì cột trái nhảy tới trang và tô sáng đoạn nguồn.
export function CitationChip({ messageId, n, source }: { messageId: string; n: number; source: Source }) {
  const active = useAskStore((s) => s.activeCitation?.messageId === messageId && s.activeCitation.ref === n);
  const openCitation = useAskStore((s) => s.openCitation);

  return (
    // className đặt trên Badge để cn() gộp đè lớp mặc định; Slot của asChild
    // chỉ nối chuỗi class.
    <Badge
      asChild
      className={cn(
        "mx-0.5 cursor-pointer gap-1 rounded-full px-[7px] py-0 align-[1px] text-meta leading-[18px] font-normal transition-colors",
        active
          ? "bg-marker-soft text-marker hover:bg-marker-soft"
          : "bg-blueprint-soft text-blueprint hover:bg-[#DCE5F3]",
      )}
    >
      <button
        type="button"
        // Chi tiết (số đoạn, câu đầu của đoạn) để trong tooltip cho chip gọn.
        title={`Trang ${source.page}${source.chunk != null ? ` · đoạn ${source.chunk + 1}` : ""}\n${source.preview}`}
        aria-label={`Nguồn ${n}: trang ${source.page}`}
        onClick={() =>
          openCitation({ messageId, ref: n, page: source.page, charStart: source.char_start, charEnd: source.char_end })
        }
      >
        <b className="font-semibold">{n}</b>
        <span className="opacity-75">tr.{source.page}</span>
      </button>
    </Badge>
  );
}
