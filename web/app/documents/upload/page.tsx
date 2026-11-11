import { UploadPanel } from "@/components/upload/upload-panel";

export default function UploadPage() {
  return (
    <div className="flex-1 overflow-y-auto px-9 py-[30px]">
      <div className="mb-[26px]">
        <h1 className="text-[25px] leading-tight font-semibold tracking-[-0.02em]">Tải tài liệu lên</h1>
        <p className="mt-[3px] max-w-[58ch] text-[14.5px] text-ink-2">
          Hệ thống trích xuất văn bản, cắt đoạn và tạo chỉ mục vector. Quá trình chạy nền, bạn có thể rời trang.
        </p>
      </div>
      <UploadPanel />
    </div>
  );
}
