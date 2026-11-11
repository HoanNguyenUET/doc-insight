import { QueryClient } from "@tanstack/react-query";

import { ApiError } from "@/lib/api";

// Query key dạng mảng, theo quy ước trong CLAUDE.md.
export const queryKeys = {
  documents: ["documents"] as const,
  document: (id: string) => ["document", id] as const,
  pages: (id: string, pageNo: number) => ["pages", id, pageNo] as const,
  chunks: (id: string) => ["chunks", id] as const,
};

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // Chuyển qua lại giữa các màn không cần hỏi lại API ngay.
        staleTime: 30_000,
        // Lỗi 4xx (không có, chưa sẵn sàng) thì thử lại cũng vô ích.
        retry: (count, err) => count < 2 && !(err instanceof ApiError && err.status >= 400 && err.status < 500),
      },
    },
  });
}
