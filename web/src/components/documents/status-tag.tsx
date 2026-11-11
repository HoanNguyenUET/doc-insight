import { Badge } from "@/components/ui/badge";
import type { DocumentStatus } from "@/lib/api";
import { statusTone, toneLabel } from "@/lib/format";
import { cn } from "@/lib/utils";

const toneClass = {
  ready: "bg-ok-soft text-ok",
  proc: "bg-warn-soft text-warn",
  err: "bg-bad-soft text-bad",
};

export function StatusTag({ status }: { status: DocumentStatus }) {
  const tone = statusTone(status);
  return (
    <Badge
      className={cn(
        "gap-[5px] rounded-full border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        toneClass[tone],
      )}
    >
      {tone === "proc" && <i className="size-[5px] animate-blip rounded-full bg-current" />}
      {toneLabel[tone]}
    </Badge>
  );
}
