import { dehydrate, HydrationBoundary } from "@tanstack/react-query";

import { DocumentLibrary } from "@/components/documents/document-library";
import { api } from "@/lib/api";
import { makeQueryClient, queryKeys } from "@/lib/query-client";

export const dynamic = "force-dynamic";

// Server Component: lấy danh sách ở server rồi chuyển cho TanStack Query ở
// client qua HydrationBoundary. API lỗi thì client tự tải lại.
export default async function DocumentsPage() {
  const queryClient = makeQueryClient();
  await queryClient.prefetchQuery({ queryKey: queryKeys.documents, queryFn: api.listDocuments });

  return (
    <div className="flex-1 overflow-y-auto px-9 py-[30px]">
      <HydrationBoundary state={dehydrate(queryClient)}>
        <DocumentLibrary
          heading={
            <div>
              <h1 className="text-[25px] leading-tight font-semibold tracking-[-0.02em]">Tài liệu</h1>
              <p className="mt-[3px] max-w-[58ch] text-[14.5px] text-ink-2">
                Các file PDF đã tải lên và đã được lập chỉ mục để hỏi đáp.
              </p>
            </div>
          }
        />
      </HydrationBoundary>
    </div>
  );
}
