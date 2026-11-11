import { useParams } from "react-router";

import { ChunkIndex } from "@/components/chunks/chunk-index";

export function ChunksPage() {
  const { id = "" } = useParams();
  return (
    <div className="flex-1 overflow-y-auto px-9 py-[30px]">
      <ChunkIndex id={id} />
    </div>
  );
}
