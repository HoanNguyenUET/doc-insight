import { Link } from "react-router";
import { useState } from "react";

import { DocumentCard } from "@/components/documents/document-card";
import { PlusIcon, SearchIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { statusTone, type StatusTone } from "@/lib/format";
import { useDocuments } from "@/lib/queries";

type Filter = "all" | StatusTone;

export function DocumentLibrary({ heading }: { heading: React.ReactNode }) {
  const { data: docs, isPending, isError, error, refetch } = useDocuments();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const q = query.trim().toLowerCase();
  const shown = docs?.filter(
    (d) => d.filename.toLowerCase().includes(q) && (filter === "all" || statusTone(d.status) === filter),
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
              <SelectItem value="proc">Đang xử lý</SelectItem>
              <SelectItem value="err">Lỗi</SelectItem>
            </SelectContent>
          </Select>
          <Button asChild>
            <Link to="/documents/upload">
              <PlusIcon className="size-[15px] stroke-2" />
              Tải lên
            </Link>
          </Button>
        </div>
      </div>

      {isPending ? (
        <Grid>
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="flex min-h-[168px] flex-col gap-3 bg-paper px-[21px] pt-5 pb-[17px]">
              <Skeleton className="h-5 w-2/3" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          ))}
        </Grid>
      ) : isError ? (
        <Empty>
          <p>Không tải được danh sách tài liệu: {error.message}</p>
          <Button variant="outline" className="mt-4" onClick={() => refetch()}>
            Thử lại
          </Button>
        </Empty>
      ) : docs.length === 0 ? (
        <Empty>
          <p>Chưa có tài liệu nào. Tải lên một file PDF để bắt đầu hỏi đáp.</p>
          <Button asChild className="mt-4">
            <Link to="/documents/upload">Tải lên</Link>
          </Button>
        </Empty>
      ) : shown!.length === 0 ? (
        <Empty>
          <p>Không có tài liệu nào khớp bộ lọc.</p>
        </Empty>
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
  return (
    <div className="grid grid-cols-[repeat(auto-fill,minmax(304px,1fr))] gap-px overflow-hidden rounded-[10px] border border-rule bg-rule">
      {children}
    </div>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-[10px] border border-rule bg-paper px-6 py-14 text-center text-sm text-ink-3">{children}</div>
  );
}
