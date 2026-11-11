import { useEffect } from "react";
import { Link, useParams } from "react-router";

import { ChatPanel } from "@/components/ask/chat-panel";
import { DocumentReader } from "@/components/ask/document-reader";
import { ReprocessButton } from "@/components/documents/reprocess-button";
import { Button } from "@/components/ui/button";
import { useAskStore } from "@/lib/ask-store";
import { statusTone } from "@/lib/format";
import { useDocument } from "@/lib/queries";

export function AskPage() {
  const { id = "" } = useParams();
  const { data: doc, isPending, error } = useDocument(id);
  const enterDocument = useAskStore((s) => s.enterDocument);

  useEffect(() => {
    enterDocument(id);
  }, [id, enterDocument]);

  if (isPending) return <Notice>Đang tải tài liệu…</Notice>;
  if (error) return <Notice back>{error.message}</Notice>;
  if (statusTone(doc.status) === "proc")
    return <Notice back>Tài liệu đang được xử lý ({doc.progress}%). Quay lại khi xử lý xong.</Notice>;
  if (doc.status === "failed")
    return (
      <Notice back action={<ReprocessButton id={doc.id} label="Thử xử lý lại" />}>
        Không đọc được tài liệu này: {doc.error}
      </Notice>
    );
  if (doc.needs_reprocess)
    return (
      <Notice back action={<ReprocessButton id={doc.id} />}>
        Tài liệu này được xử lý bằng một mô hình AI cũ nên AI sẽ tìm sai đoạn khi trả lời. Bấm "Xử lý lại" để đọc lại tài
        liệu bằng mô hình hiện tại (mất vài giây đến vài phút).
      </Notice>
    );

  return (
    <div className="flex flex-1 overflow-hidden">
      <DocumentReader doc={doc} />
      <ChatPanel doc={doc} />
    </div>
  );
}

function Notice({ children, back, action }: { children: React.ReactNode; back?: boolean; action?: React.ReactNode }) {
  return (
    <div className="m-auto max-w-[44ch] px-5 text-center text-sm leading-relaxed text-ink-3">
      <p>{children}</p>
      <div className="mt-4 flex justify-center gap-2">
        {action}
        {back && (
          <Button asChild variant="outline">
            <Link to="/documents">Về danh sách tài liệu</Link>
          </Button>
        )}
      </div>
    </div>
  );
}
