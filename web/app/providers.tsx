"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { useLayoutEffect, useState } from "react";

import { useAskStore } from "@/lib/ask-store";
import { makeQueryClient } from "@/lib/query-client";

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(makeQueryClient);

  // Store bỏ qua hydrate lúc tạo để không lệch với HTML server. Phải là
  // layout effect: nó chạy trước mọi useEffect của component con. Nếu con
  // gọi set() trước khi rehydrate, persist sẽ ghi state rỗng đè lên
  // localStorage và mất hết hội thoại.
  useLayoutEffect(() => {
    void useAskStore.persist.rehydrate();
  }, []);

  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
