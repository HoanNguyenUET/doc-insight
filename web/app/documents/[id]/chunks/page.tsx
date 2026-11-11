import { dehydrate, HydrationBoundary } from "@tanstack/react-query";

import { ChunkIndex } from "@/components/chunks/chunk-index";
import { api } from "@/lib/api";
import { makeQueryClient, queryKeys } from "@/lib/query-client";

export const dynamic = "force-dynamic";

export default async function ChunksPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const queryClient = makeQueryClient();
  await Promise.all([
    queryClient.prefetchQuery({ queryKey: queryKeys.document(id), queryFn: () => api.getDocument(id) }),
    queryClient.prefetchQuery({ queryKey: queryKeys.chunks(id), queryFn: () => api.listChunks(id) }),
  ]);

  return (
    <div className="flex-1 overflow-y-auto px-9 py-[30px]">
      <HydrationBoundary state={dehydrate(queryClient)}>
        <ChunkIndex id={id} />
      </HydrationBoundary>
    </div>
  );
}
