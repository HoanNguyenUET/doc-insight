import { UploadPanel } from "@/components/upload/upload-panel";

export function UploadPage() {
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-[1180px] px-8 py-8">
        <div className="mb-6">
          <h1 className="font-serif text-display leading-tight font-semibold tracking-[-0.01em]">Tải tài liệu lên</h1>
          <p className="mt-1 max-w-[60ch] text-body text-ink-2">
            Chọn file PDF. AI sẽ đọc tài liệu trong vài giây đến vài phút; bạn có thể rời trang trong lúc đó.
          </p>
        </div>
        <UploadPanel />
      </div>
    </div>
  );
}
