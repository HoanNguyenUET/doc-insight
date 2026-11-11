import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Document } from "@/lib/api";
import { isProcessing } from "@/lib/document-state";
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

// useReprocessDocument xử lý lại tài liệu từ file gốc; tài liệu về trạng
// thái đang xử lý nên useDocument/useDocuments tự hỏi lại tiến độ.
export function useReprocessDocument() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.reprocessDocument,
    onSuccess: (doc) => {
      qc.setQueryData(queryKeys.document(doc.id), doc);
      void qc.invalidateQueries({ queryKey: queryKeys.documents });
      qc.removeQueries({ queryKey: ["pages", doc.id] });
      qc.removeQueries({ queryKey: queryKeys.chunks(doc.id) });
    },
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
