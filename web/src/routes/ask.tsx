import { ChatPanel } from "@/components/ask/chat-panel";
import { DocumentReader } from "@/components/ask/document-reader";
import { useCurrentDocument } from "@/routes/use-current-document";

// Tab Hỏi đáp: trang PDF bên trái, khung chat bên phải.
export function AskPage() {
  const doc = useCurrentDocument();
  return (
    <div className="flex flex-1 overflow-hidden">
      <DocumentReader doc={doc} />
      <ChatPanel doc={doc} />
    </div>
  );
}
