import { Link } from "react-router";
import { useState } from "react";

import { DocumentCard } from "@/components/documents/document-card";
import { EmptyState } from "@/components/empty-state";
import { SearchIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { documentState, type StateKind } from "@/lib/document-state";
import { useDocuments } from "@/lib/queries";

type Filter = "all" | StateKind;

export function DocumentLibrary({ heading }: { heading: React.ReactNode }) {
  const { data: docs, isPending, isError, error, refetch } = useDocuments();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const q = query.trim().toLowerCase();
  const shown = docs?.filter(
    (d) => d.filename.toLowerCase().includes(q) && (filter === "all" || documentState(d).kind === filter),
  );

  return (
    <>
      <div className="mb-[26px] flex flex-wrap items-end justify-between gap-6">
        {heading}
        <div className="flex items-center gap-[9px]">
          <div className="relative">
            <SearchIcon className="absolute top-1/2 left-2.5 size-[15px] -translate-y-1/2 stroke-[1.8] text-ink-3" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Tìm tài liệu"
              className="w-[210px] bg-paper pl-[31px]"
              aria-label="Tìm tài liệu"
            />
          </div>
          <Select value={filter} onValueChange={(v) => setFilter(v as Filter)}>
            <SelectTrigger className="w-[160px] bg-paper" aria-label="Lọc theo trạng thái">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Mọi trạng thái</SelectItem>
              <SelectItem value="ready">Sẵn sàng</SelectItem>
              <SelectItem value="processing">Đang xử lý</SelectItem>
              <SelectItem value="stale">Cần xử lý lại</SelectItem>
              <SelectItem value="failed">Lỗi</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {isPending ? (
        <Grid>
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="flex min-h-[132px] flex-col gap-3 rounded-card border border-rule bg-paper p-4">
              <Skeleton className="h-5 w-2/3" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          ))}
        </Grid>
      ) : isError ? (
        <EmptyState
          framed
          action={
            <Button variant="outline" onClick={() => refetch()}>
              Thử lại
            </Button>
          }
        >
          Không tải được danh sách tài liệu: {error.message}
        </EmptyState>
      ) : docs.length === 0 ? (
        <EmptyState
          framed
          action={
            <Button asChild>
              <Link to="/documents/upload">Tải lên</Link>
            </Button>
          }
        >
          Chưa có tài liệu nào. Tải lên một file PDF để bắt đầu hỏi đáp.
        </EmptyState>
      ) : shown!.length === 0 ? (
        <EmptyState framed>Không có tài liệu nào khớp bộ lọc.</EmptyState>
      ) : (
        <Grid>
          {shown!.map((d) => (
            <DocumentCard key={d.id} doc={d} />
          ))}
        </Grid>
      )}
    </>
  );
}

function Grid({ children }: { children: React.ReactNode }) {
  return <div className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3.5">{children}</div>;
}
