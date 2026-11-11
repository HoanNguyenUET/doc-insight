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
        "mx-px cursor-pointer gap-[5px] rounded-[5px] py-px pr-[7px] pl-[5px] align-baseline text-[12.5px] transition-colors",
        active ? "bg-marker-soft text-marker hover:bg-marker-soft" : "bg-blueprint-soft text-blueprint hover:bg-[#DCE5F3]",
      )}
    >
      <button
        type="button"
        title={source.preview}
        onClick={() =>
          openCitation({ messageId, ref: n, page: source.page, charStart: source.char_start, charEnd: source.char_end })
        }
      >
        <b className="grid size-3.5 place-items-center rounded-[3px] bg-current font-mono text-[10px]">
          <span className={active ? "text-marker-soft" : "text-blueprint-soft"}>{n}</span>
        </b>
        Trang {source.page}
        {source.chunk != null && <span className="opacity-70">· đoạn {source.chunk + 1}</span>}
      </button>
    </Badge>
  );
}
