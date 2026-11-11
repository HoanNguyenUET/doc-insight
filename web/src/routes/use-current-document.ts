import { useOutletContext } from "react-router";

import type { Document } from "@/lib/api";

// useCurrentDocument trả tài liệu (đã sẵn sàng) mà DocumentLayout truyền cho
// các tab con qua Outlet.
export function useCurrentDocument() {
  return useOutletContext<Document>();
}
