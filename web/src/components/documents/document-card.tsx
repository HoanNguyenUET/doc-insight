import { Link } from "react-router";
import { toast } from "sonner";

import { StatusTag } from "@/components/documents/status-tag";
import { useOpenDocument } from "@/components/documents/use-open-document";
import { FileIcon, IndexIcon, TrashIcon } from "@/components/icons";
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
import { documentState } from "@/lib/document-state";
import { displayName, formatBytes, formatDate } from "@/lib/format";
import { useDeleteDocument } from "@/lib/queries";
import { cn } from "@/lib/utils";

// Cả thẻ bấm được, mở tài liệu qua useOpenDocument. Nhãn trạng thái chỉ hiện
// khi cần chú ý (đang xử lý, lỗi, cần xử lý lại); nút thao tác hiện khi rê chuột.
export function DocumentCard({ doc }: { doc: Document }) {
  const openDocument = useOpenDocument();
  const state = documentState(doc);
  // Tài liệu cần xử lý lại vẫn mở được (màn đó có nút Xử lý lại).
  const ready = state.kind === "ready" || state.kind === "stale";
  const convCount = useAskStore((s) => s.conversations.filter((c) => c.docId === doc.id).length);

  const meta = [
    doc.page_count != null && `${doc.page_count} trang`,
    formatBytes(doc.size_bytes),
    formatDate(doc.created_at),
  ].filter(Boolean);

  return (
    <div
      role="link"
      tabIndex={0}
      aria-disabled={!ready}
      onClick={() => openDocument(doc)}
      onKeyDown={(e) => {
        if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          openDocument(doc);
        }
      }}
      className={cn(
        "group flex flex-col rounded-card border border-rule bg-paper p-4 text-left transition-[border-color,box-shadow]",
        ready ? "cursor-pointer hover:border-ink-3/40 hover:shadow-card" : "cursor-default",
      )}
    >
      <div className="flex items-start gap-3">
        <span className="grid size-10 flex-none place-items-center rounded-control bg-field text-ink-3 transition-colors group-hover:text-blueprint">
          <FileIcon className="size-5 stroke-[1.5]" />
        </span>
        <div className="min-w-0 flex-1">
          <h3
            className={cn(
              "line-clamp-2 font-serif text-title leading-snug font-semibold break-words transition-colors",
              ready && "group-hover:text-blueprint",
            )}
          >
            {displayName(doc.filename)}
          </h3>
          <p className="mt-0.5 text-caption text-ink-3">{meta.join(" · ")}</p>
        </div>
        {state.kind !== "ready" && <StatusTag doc={doc} />}
      </div>

      {state.kind === "processing" && (
        <div className="mt-3.5 h-1 overflow-hidden rounded-full bg-rule-soft">
          <i className="block h-full rounded-full bg-marker transition-[width]" style={{ width: `${doc.progress}%` }} />
        </div>
      )}

      <div className="mt-4 flex min-h-[26px] items-center justify-between gap-2">
        <span className="text-caption text-ink-3">
          {state.kind !== "ready" ? "" : convCount ? `${convCount} cuộc hỏi đáp` : "Chưa có câu hỏi nào"}
        </span>
        <div
          className="flex gap-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100"
          onClick={(e) => e.stopPropagation()}
        >
          {ready && (
            <Button asChild variant="ghost" size="icon" className="size-[26px] text-ink-3 hover:bg-field">
              <Link to={`/documents/${doc.id}/content`} aria-label="Xem nội dung" title="Xem nội dung tài liệu">
                <IndexIcon className="size-4 stroke-[1.7]" />
              </Link>
            </Button>
          )}
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
