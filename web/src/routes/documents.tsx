import { DocumentLibrary } from "@/components/documents/document-library";

export function DocumentsPage() {
  return (
    <div className="flex-1 overflow-y-auto px-9 py-[30px]">
      <DocumentLibrary
        heading={
          <div>
            <h1 className="text-[25px] leading-tight font-semibold tracking-[-0.02em]">Tài liệu</h1>
            <p className="mt-[3px] max-w-[58ch] text-[14.5px] text-ink-2">
              Các file PDF bạn đã tải lên. Bấm vào một tài liệu để đặt câu hỏi về nó.
            </p>
          </div>
        }
      />
    </div>
  );
}
