import { useNavigate } from "react-router";
import { toast } from "sonner";

import type { Document } from "@/lib/api";
import { documentState } from "@/lib/document-state";

// useOpenDocument: mở màn của tài liệu; đang xử lý hoặc lỗi thì chỉ báo toast
// (tài liệu cần xử lý lại vẫn mở được, màn đó có nút Xử lý lại). Dùng chung cho
// thẻ ở danh sách và thanh bên trái.
export function useOpenDocument() {
  const navigate = useNavigate();
  return (doc: Document) => {
    const state = documentState(doc);
    if (state.kind === "processing") toast("Tài liệu đang được xử lý", { description: `Đã xong ${doc.progress}%` });
    else if (state.kind === "failed") toast("Không đọc được tài liệu này", { description: doc.error ?? undefined });
    else navigate(`/documents/${doc.id}`);
  };
}
