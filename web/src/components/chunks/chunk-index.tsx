import { Link, useNavigate } from "react-router";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { Chunk } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { formatBytes, processingSeconds } from "@/lib/format";
import { useChunks, useDocument } from "@/lib/queries";
import { cn } from "@/lib/utils";

// Màn chỉ mục: các đoạn đã cắt của một tài liệu.
export function ChunkIndex({ id }: { id: string }) {
  const { data: doc, error: docError } = useDocument(id);
  const ready = doc?.status === "ready";
  const { data: chunks, error: chunksError, isPending } = useChunks(id, ready);
  const [open, setOpen] = useState<Set<number>>(new Set());

  const toggle = (chunkId: number) =>
    setOpen((s) => {
      const next = new Set(s);
      if (!next.delete(chunkId)) next.add(chunkId);
      return next;
    });

  const seconds = doc ? processingSeconds(doc) : null;
  const facts = [
    { label: "Số trang", value: doc?.page_count ?? "—", mono: true },
    { label: "Số đoạn", value: doc?.chunk_count ?? "—", mono: true },
    { label: "Thời gian xử lý", value: seconds == null ? "—" : `${seconds < 10 ? seconds.toFixed(1) : Math.round(seconds)} giây`, mono: true },
    { label: "Dung lượng", value: doc ? formatBytes(doc.size_bytes) : "—", mono: true },
  ];

  const error = docError ?? chunksError;

  return (
    <>
      <div className="mb-[26px] flex flex-wrap items-end justify-between gap-6">
        <div>
          <h1 className="text-[25px] leading-tight font-semibold tracking-[-0.02em]">Chỉ mục tài liệu</h1>
          <p className="mt-[3px] max-w-[58ch] text-[14.5px] text-ink-2">
            {doc ? `${doc.filename} — các đoạn đã cắt, kèm vị trí trong trang.` : "Đang tải…"}
          </p>
        </div>
        <div className="flex gap-[9px]">
          {ready && (
            <Button asChild>
              <Link to={`/documents/${id}`}>Hỏi đáp</Link>
            </Button>
          )}
          <Button asChild variant="outline" className="bg-paper">
            <Link to="/documents">Quay lại</Link>
          </Button>
        </div>
      </div>

      <dl className="mb-[26px] grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-px overflow-hidden rounded-[10px] border border-rule bg-rule">
        {facts.map((f) => (
          <div key={f.label} className="bg-paper px-[18px] py-[15px]">
            <dt className="mb-[3px] text-[13px] text-ink-3">{f.label}</dt>
            <dd className={cn("text-[15.5px] font-medium", f.mono && "font-mono text-sm")}>{f.value}</dd>
          </div>
        ))}
      </dl>

      <section className="overflow-hidden rounded-[10px] border border-rule bg-paper">
        <div className="flex items-center justify-between border-b border-rule-soft px-[18px] py-3.5">
          <h3 className="text-[15px] font-semibold">Các đoạn đã cắt</h3>
          <span className="text-[13px] text-ink-3">Bấm một dòng để xem đầy đủ</span>
        </div>
        {error ? (
          <p className="px-[18px] py-10 text-center text-sm text-bad">{error.message}</p>
        ) : doc && !ready ? (
          <p className="px-[18px] py-10 text-center text-sm text-ink-3">Tài liệu chưa xử lý xong, chưa có đoạn nào.</p>
        ) : isPending || !chunks ? (
          <p className="px-[18px] py-10 text-center text-sm text-ink-3">Đang tải…</p>
        ) : (
          <Table className="text-sm">
            <TableHeader>
              <TableRow className="border-rule hover:bg-transparent">
                <TableHead className="w-[92px] px-[18px] text-[13px] font-medium text-ink-3">Mã</TableHead>
                <TableHead className="w-[66px] px-[18px] text-[13px] font-medium text-ink-3">Trang</TableHead>
                <TableHead className="px-[18px] text-[13px] font-medium text-ink-3">Nội dung</TableHead>
                <TableHead className="w-[78px] px-[18px] text-[13px] font-medium text-ink-3">Ký tự</TableHead>
                <TableHead className="w-[80px] px-[18px] text-[13px] font-medium text-ink-3">Token</TableHead>
                <TableHead className="w-[64px] px-[18px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {chunks.map((c) => (
                <ChunkRow key={c.id} docId={id} chunk={c} open={open.has(c.id)} onToggle={() => toggle(c.id)} />
              ))}
            </TableBody>
          </Table>
        )}
      </section>
    </>
  );
}

function ChunkRow({ docId, chunk: c, open, onToggle }: { docId: string; chunk: Chunk; open: boolean; onToggle: () => void }) {
  const navigate = useNavigate();
  const jumpTo = useAskStore((s) => s.jumpTo);

  // Mở màn hỏi đáp ở đúng trang và tô sáng đoạn này.
  const view = () => {
    jumpTo(docId, { messageId: `chunk-${c.id}`, ref: 0, page: c.page_number, charStart: c.char_start, charEnd: c.char_end });
    navigate(`/documents/${docId}`);
  };

  return (
    <TableRow onClick={onToggle} className="cursor-pointer border-rule-soft align-top hover:bg-white">
      <TableCell className="px-[18px] py-[13px] font-mono text-[12.5px] whitespace-nowrap text-ink-2">
        CHK-{String(c.index + 1).padStart(3, "0")}
      </TableCell>
      <TableCell className="px-[18px] py-[13px] font-mono text-[12.5px] text-ink-2">{c.page_number}</TableCell>
      <TableCell className={cn("px-[18px] py-[13px] leading-normal whitespace-normal", open ? "text-ink" : "text-ink-2")}>
        <span className={cn(open ? "whitespace-pre-wrap" : "line-clamp-2")}>{c.content}</span>
      </TableCell>
      <TableCell className="px-[18px] py-[13px] font-mono text-[12.5px] text-ink-2">{c.char_end - c.char_start}</TableCell>
      <TableCell className="px-[18px] py-[13px] font-mono text-[12.5px] text-ink-2">{c.token_count ?? "—"}</TableCell>
      <TableCell className="px-[18px] py-[9px]">
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-[13px] text-blueprint hover:bg-field"
          onClick={(e) => {
            e.stopPropagation();
            view();
          }}
        >
          Xem
        </Button>
      </TableCell>
    </TableRow>
  );
}
