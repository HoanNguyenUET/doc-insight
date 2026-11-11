import { useEffect } from "react";
import { NavLink, Outlet, useParams } from "react-router";

import { ReprocessButton } from "@/components/documents/reprocess-button";
import { EmptyState } from "@/components/empty-state";
import { useAskStore } from "@/lib/ask-store";
import type { Document } from "@/lib/api";
import { documentState } from "@/lib/document-state";
import { displayName, formatBytes } from "@/lib/format";
import { useDocument } from "@/lib/queries";
import { cn } from "@/lib/utils";

// Khung chung của một tài liệu: thanh tiêu đề với tab Hỏi đáp | Nội dung, và
// các trạng thái chưa dùng được (đang tải, đang xử lý, lỗi, cần xử lý lại).
// Tab con chỉ hiện khi tài liệu sẵn sàng, nhận tài liệu qua useCurrentDocument.
export function DocumentLayout() {
  const { id = "" } = useParams();
  const { data: doc, isPending, error } = useDocument(id);
  const enterDocument = useAskStore((s) => s.enterDocument);

  useEffect(() => {
    enterDocument(id);
  }, [id, enterDocument]);

  let body: React.ReactNode;
  if (isPending) body = <EmptyState>Đang tải tài liệu…</EmptyState>;
  else if (error) body = <EmptyState back>{error.message}</EmptyState>;
  else body = <DocumentBody doc={doc} />;

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <header className="flex h-[60px] flex-none items-center gap-4 border-b border-rule bg-paper px-6">
        <div className="min-w-0 flex-1">
          <h1 className="truncate font-serif text-heading leading-tight font-semibold">
            {doc ? displayName(doc.filename) : " "}
          </h1>
          {doc && (
            <p className="truncate text-caption text-ink-3">
              {[doc.page_count != null && `${doc.page_count} trang`, formatBytes(doc.size_bytes)]
                .filter(Boolean)
                .join(" · ")}
            </p>
          )}
        </div>
        <nav className="flex flex-none gap-0.5 rounded-control bg-field p-[3px]" aria-label="Chế độ xem">
          <Tab to={`/documents/${id}`} end>
            Hỏi đáp
          </Tab>
          <Tab to={`/documents/${id}/content`}>Nội dung</Tab>
        </nav>
      </header>
      {body}
    </div>
  );
}

function Tab({ to, end, children }: { to: string; end?: boolean; children: React.ReactNode }) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          "rounded-control-sm px-3.5 py-1.5 text-ui transition-colors",
          isActive ? "bg-paper font-medium text-ink shadow-tab" : "text-ink-2 hover:text-ink",
        )
      }
    >
      {children}
    </NavLink>
  );
}

// DocumentBody: tab con khi tài liệu sẵn sàng, còn lại là thông báo trạng thái.
function DocumentBody({ doc }: { doc: Document }) {
  switch (documentState(doc).kind) {
    case "processing":
      return (
        <EmptyState>
          <span className="mb-3 block text-ink-2">Đang đọc tài liệu ({doc.progress}%)…</span>
          <span className="mx-auto block h-1.5 w-56 overflow-hidden rounded-full bg-rule-soft">
            <span
              className="block h-full rounded-full bg-marker transition-[width]"
              style={{ width: `${doc.progress}%` }}
            />
          </span>
        </EmptyState>
      );
    case "failed":
      return (
        <EmptyState back action={<ReprocessButton id={doc.id} label="Thử xử lý lại" />}>
          Không đọc được tài liệu này: {doc.error}
        </EmptyState>
      );
    case "stale":
      return (
        <EmptyState back action={<ReprocessButton id={doc.id} />}>
          Tài liệu này được xử lý bằng một mô hình AI cũ nên AI sẽ tìm sai đoạn khi trả lời. Bấm "Xử lý lại" để đọc lại
          tài liệu bằng mô hình hiện tại (mất vài giây đến vài phút).
        </EmptyState>
      );
    case "ready":
      return <Outlet context={doc} />;
  }
}
