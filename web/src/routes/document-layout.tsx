import { useEffect } from "react";
import { Link, NavLink, Outlet, useParams } from "react-router";

import { ReprocessButton } from "@/components/documents/reprocess-button";
import { Button } from "@/components/ui/button";
import { useAskStore } from "@/lib/ask-store";
import { displayName, formatBytes, statusTone } from "@/lib/format";
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
  if (isPending) body = <Notice>Đang tải tài liệu…</Notice>;
  else if (error) body = <Notice back>{error.message}</Notice>;
  else if (statusTone(doc.status) === "proc")
    body = (
      <Notice>
        <span className="mb-3 block text-ink-2">Đang đọc tài liệu ({doc.progress}%)…</span>
        <span className="mx-auto block h-1.5 w-56 overflow-hidden rounded-full bg-rule-soft">
          <span
            className="block h-full rounded-full bg-marker transition-[width]"
            style={{ width: `${doc.progress}%` }}
          />
        </span>
      </Notice>
    );
  else if (doc.status === "failed")
    body = (
      <Notice back action={<ReprocessButton id={doc.id} label="Thử xử lý lại" />}>
        Không đọc được tài liệu này: {doc.error}
      </Notice>
    );
  else if (doc.needs_reprocess)
    body = (
      <Notice back action={<ReprocessButton id={doc.id} />}>
        Tài liệu này được xử lý bằng một mô hình AI cũ nên AI sẽ tìm sai đoạn khi trả lời. Bấm "Xử lý lại" để đọc lại
        tài liệu bằng mô hình hiện tại (mất vài giây đến vài phút).
      </Notice>
    );
  else body = <Outlet context={doc} />;

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

function Notice({ children, back, action }: { children: React.ReactNode; back?: boolean; action?: React.ReactNode }) {
  return (
    <div className="m-auto max-w-[46ch] px-5 text-center text-sm leading-relaxed text-ink-3">
      <p>{children}</p>
      {(action || back) && (
        <div className="mt-4 flex justify-center gap-2">
          {action}
          {back && (
            <Button asChild variant="outline">
              <Link to="/documents">Về danh sách tài liệu</Link>
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
