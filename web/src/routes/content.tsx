import { DocumentContent } from "@/components/content/document-content";
import { useCurrentDocument } from "@/routes/use-current-document";

// Tab Nội dung: các đoạn AI đã đọc từ tài liệu.
export function ContentPage() {
  const doc = useCurrentDocument();
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-[880px] px-8 py-8">
        <DocumentContent id={doc.id} />
      </div>
    </div>
  );
}
