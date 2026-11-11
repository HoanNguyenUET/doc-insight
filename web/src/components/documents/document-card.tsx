import { Link } from "react-router";
import { toast } from "sonner";

import { StatusTag } from "@/components/documents/status-tag";
import { useOpenDocument } from "@/components/documents/use-open-document";
import { IndexIcon, TrashIcon } from "@/components/icons";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import type { Document } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { displayName, formatBytes, formatDate, statusTone } from "@/lib/format";
import { useDeleteDocument } from "@/lib/queries";
import { cn } from "@/lib/utils";

// Cả thẻ bấm được, mở tài liệu qua useOpenDocument.
export function DocumentCard({ doc }: { doc: Document }) {
  const openDocument = useOpenDocument();
  const tone = statusTone(doc.status);
  const ready = tone === "ready";
  const convCount = useAskStore((s) => s.conversations.filter((c) => c.docId === doc.id).length);

  const open = () => openDocument(doc);

  const meta = [
    formatBytes(doc.size_bytes),
    doc.page_count != null && `${doc.page_count} trang`,
    doc.chunk_count > 0 && `${doc.chunk_count} đoạn`,
    formatDate(doc.created_at),
  ].filter(Boolean);

  return (
    <div
      role="link"
      tabIndex={0}
      aria-disabled={!ready}
      onClick={open}
      onKeyDown={(e) => {
        if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          open();
        }
      }}
      className={cn(
        "group flex min-h-[168px] w-full flex-col bg-paper px-[21px] pt-5 pb-[17px] text-left transition-colors",
        ready ? "cursor-pointer hover:bg-white" : "cursor-default",
      )}
    >
      <div className="mb-[11px] flex items-start justify-between gap-3">
        <h3
          className={cn(
            "font-serif text-[17.5px] leading-[1.3] font-semibold tracking-[-0.005em] break-words transition-colors",
            ready && "group-hover:text-blueprint",
          )}
        >
          {displayName(doc.filename)}
        </h3>
        <StatusTag status={doc.status} />
      </div>
      <div className="mb-3.5 font-mono text-[11.5px] tracking-[-0.01em] text-ink-3">{meta.join(" · ")}</div>
      {tone === "proc" && (
        <div className="mb-3 h-[3px] overflow-hidden rounded-sm bg-rule-soft">
          <i className="block h-full rounded-sm bg-marker transition-[width]" style={{ width: `${doc.progress}%` }} />
        </div>
      )}
      <div className="mt-auto flex items-center justify-between border-t border-rule-soft pt-[13px]">
        <span className="text-[13px] text-ink-3">
          {!ready ? " " : convCount ? `${convCount} hội thoại` : "Chưa có hội thoại"}
        </span>
        <div className="flex gap-0.5" onClick={(e) => e.stopPropagation()}>
          {ready ? (
            <Button asChild variant="ghost" size="icon" className="size-[26px] text-ink-3 hover:bg-field">
              <Link to={`/documents/${doc.id}/chunks`} aria-label="Xem nội dung" title="Xem nội dung tài liệu">
                <IndexIcon className="size-4 stroke-[1.7]" />
              </Link>
            </Button>
          ) : null}
          <DeleteButton doc={doc} />
        </div>
      </div>
    </div>
  );
}

function DeleteButton({ doc }: { doc: Document }) {
  const del = useDeleteDocument();
  const removeConversations = useAskStore((s) => s.removeDocument);

  const confirm = () =>
    del.mutate(doc.id, {
      onSuccess: () => {
        removeConversations(doc.id);
        toast("Đã xoá tài liệu");
      },
      onError: (err) => toast.error("Không xoá được tài liệu", { description: err.message }),
    });

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-[26px] text-ink-3 hover:bg-field"
          aria-label="Xoá"
          title="Xoá"
          disabled={del.isPending}
        >
          <TrashIcon className="size-4 stroke-[1.7]" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Xoá tài liệu?</AlertDialogTitle>
          <AlertDialogDescription>
            “{doc.filename}” cùng toàn bộ đoạn đã cắt và các hội thoại về tài liệu này sẽ bị xoá. Không hoàn tác được.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Huỷ</AlertDialogCancel>
          <AlertDialogAction onClick={confirm} className="bg-bad text-white hover:bg-bad/90">
            Xoá
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
