import { Badge } from "@/components/ui/badge";
import type { Document } from "@/lib/api";
import { documentState } from "@/lib/document-state";
import { cn } from "@/lib/utils";

const toneClass = {
  ok: "bg-ok-soft text-ok",
  warn: "bg-warn-soft text-warn",
  bad: "bg-bad-soft text-bad",
};

// StatusTag: nhãn trạng thái của tài liệu (chấm nhấp nháy khi đang xử lý).
export function StatusTag({ doc }: { doc: Document }) {
  const state = documentState(doc);
  return (
    <Badge
      className={cn(
        "gap-[5px] rounded-full border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        toneClass[state.tone],
      )}
    >
      {state.kind === "processing" && <i className="size-[5px] animate-blip rounded-full bg-current" />}
      {state.kind === "processing" ? "Đang xử lý" : state.label}
    </Badge>
  );
}
