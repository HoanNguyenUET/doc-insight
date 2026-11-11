import { useEffect } from "react";
import { Link, useParams } from "react-router";

import { ChatPanel } from "@/components/ask/chat-panel";
import { DocumentReader } from "@/components/ask/document-reader";
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
  if (doc.status === "failed") return <Notice back>Không đọc được tài liệu này: {doc.error}</Notice>;

  return (
    <div className="flex flex-1 overflow-hidden">
      <DocumentReader doc={doc} />
      <ChatPanel doc={doc} />
    </div>
  );
}

function Notice({ children, back }: { children: React.ReactNode; back?: boolean }) {
  return (
    <div className="m-auto max-w-[40ch] px-5 text-center text-sm leading-relaxed text-ink-3">
      <p>{children}</p>
      {back && (
        <Button asChild variant="outline" className="mt-4">
          <Link to="/documents">Về danh sách tài liệu</Link>
        </Button>
      )}
    </div>
  );
}
