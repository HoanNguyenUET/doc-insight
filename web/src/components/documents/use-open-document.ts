import { useNavigate } from "react-router";
import { toast } from "sonner";

import type { Document } from "@/lib/api";
import { statusTone } from "@/lib/format";

// useOpenDocument: tài liệu sẵn sàng thì mở màn hỏi đáp, đang xử lý hoặc lỗi
// thì chỉ báo toast. Dùng chung cho thẻ ở danh sách và thanh bên trái.
export function useOpenDocument() {
  const navigate = useNavigate();
  return (doc: Document) => {
    const tone = statusTone(doc.status);
    if (tone === "proc") toast("Tài liệu đang được xử lý", { description: `Đã xong ${doc.progress}%` });
    else if (tone === "err") toast("Không đọc được tài liệu này", { description: doc.error ?? undefined });
    else navigate(`/documents/${doc.id}`);
  };
}
