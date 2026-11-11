import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, isProcessing, type Document } from "@/lib/api";
import { queryKeys } from "@/lib/query-client";

const POLL_MS = 1500;

export function useDocuments() {
  return useQuery({
    queryKey: queryKeys.documents,
    queryFn: api.listDocuments,
    // Còn tài liệu đang xử lý thì hỏi lại để cập nhật thanh tiến độ.
    refetchInterval: (q) => (q.state.data?.some((d) => isProcessing(d.status)) ? POLL_MS : false),
  });
}

export function useDocument(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.document(id ?? ""),
    queryFn: () => api.getDocument(id!),
    enabled: !!id,
    refetchInterval: (q) => (q.state.data && isProcessing(q.state.data.status) ? POLL_MS : false),
  });
}

export function usePage(id: string, pageNo: number, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.pages(id, pageNo),
    queryFn: () => api.getPage(id, pageNo),
    enabled,
    // Văn bản trang không đổi sau khi xử lý xong.
    staleTime: Infinity,
    // Giữ trang cũ trên màn hình trong lúc tải trang mới, khỏi nháy trắng.
    placeholderData: keepPreviousData,
  });
}

export function useChunks(id: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.chunks(id),
    queryFn: () => api.listChunks(id),
    enabled,
    staleTime: Infinity,
  });
}

export function useDeleteDocument() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.deleteDocument,
    onSuccess: (_, id) => {
      qc.setQueryData<Document[]>(queryKeys.documents, (docs) => docs?.filter((d) => d.id !== id));
      qc.removeQueries({ queryKey: queryKeys.document(id) });
      qc.removeQueries({ queryKey: ["pages", id] });
      qc.removeQueries({ queryKey: queryKeys.chunks(id) });
    },
  });
}
