import { DocumentLibrary } from "@/components/documents/document-library";

export function DocumentsPage() {
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-[1180px] px-8 py-8">
        <DocumentLibrary
          heading={
            <div>
              <h1 className="font-serif text-display leading-tight font-semibold tracking-[-0.01em]">Tài liệu</h1>
              <p className="mt-[3px] max-w-[58ch] text-body text-ink-2">
                Các file PDF bạn đã tải lên. Bấm vào một tài liệu để đặt câu hỏi về nó.
              </p>
            </div>
          }
        />
      </div>
    </div>
  );
}
