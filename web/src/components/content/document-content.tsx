import { useMemo, useState } from "react";
import { useNavigate } from "react-router";

import { SearchIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Chunk } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { formatBytes, processingSeconds } from "@/lib/format";
import { useChunks, useDocument } from "@/lib/queries";
import { cn } from "@/lib/utils";

// Bỏ dấu và chữ hoa để tìm "phan boi chau" ra "Phan Bội Châu".
const fold = (s: string) =>
  s.normalize("NFD").replace(/\p{M}/gu, "").replace(/đ/g, "d").replace(/Đ/g, "D").toLowerCase();

// Màn "Nội dung tài liệu": giải thích bằng lời thường AI đã đọc tài liệu thế
// nào, rồi liệt kê các đoạn theo trang. Số liệu kỹ thuật (ký tự, token) chỉ
// hiện khi bật.
export function DocumentContent({ id }: { id: string }) {
  const { data: doc, error: docError } = useDocument(id);
  const ready = doc?.status === "ready";
  const { data: chunks, error: chunksError, isPending } = useChunks(id, ready);
  const [query, setQuery] = useState("");
  const [technical, setTechnical] = useState(false);

  const pages = useMemo(() => {
    const q = fold(query.trim());
    const byPage = new Map<number, Chunk[]>();
    for (const c of chunks ?? []) {
      if (q && !fold(c.content).includes(q)) continue;
      byPage.set(c.page_number, [...(byPage.get(c.page_number) ?? []), c]);
    }
    return [...byPage.entries()].sort(([a], [b]) => a - b);
  }, [chunks, query]);

  const seconds = doc ? processingSeconds(doc) : null;
  const error = docError ?? chunksError;

  return (
    <>
      {doc && ready && (
        <section className="mb-6 rounded-[12px] border border-rule bg-paper px-5 py-[18px]">
          <h2 className="mb-3.5 text-[15px] font-semibold">AI dùng tài liệu này thế nào?</h2>
          <ol className="grid gap-3 xl:grid-cols-3">
            <Step n={1} title={`Đọc ${doc.page_count ?? 0} trang`}>
              Lấy toàn bộ chữ trong file PDF ({formatBytes(doc.size_bytes)})
              {seconds != null &&
                `, mất ${seconds < 10 ? seconds.toFixed(1).replace(".", ",") : Math.round(seconds)} giây`}
              .
            </Step>
            <Step n={2} title={`Chia thành ${doc.chunk_count} đoạn`}>
              Mỗi đoạn dài khoảng nửa trang, để tìm nhanh phần liên quan tới câu hỏi.
            </Step>
            <Step n={3} title="Trả lời kèm nguồn">
              Khi bạn hỏi, AI đọc vài đoạn khớp nhất rồi trả lời và ghi rõ đoạn đó nằm ở trang nào.
            </Step>
          </ol>
        </section>
      )}

      {error ? (
        <Panel tone="bad">{error.message}</Panel>
      ) : doc && !ready ? (
        <Panel>Tài liệu chưa xử lý xong, chưa có nội dung để xem.</Panel>
      ) : isPending || !chunks ? (
        <Panel>Đang tải…</Panel>
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-3">
            <div className="relative min-w-[240px] flex-1">
              <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-3" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Tìm chữ trong tài liệu, ví dụ: hồ sơ, thời hạn…"
                className="h-10 bg-paper pl-9"
              />
            </div>
            <label className="flex cursor-pointer items-center gap-2 text-[13px] text-ink-3 select-none">
              <input
                type="checkbox"
                checked={technical}
                onChange={(e) => setTechnical(e.target.checked)}
                className="accent-blueprint"
              />
              Hiện thông tin kỹ thuật
            </label>
          </div>

          {pages.length === 0 ? (
            <Panel>Không có đoạn nào chứa “{query}”.</Panel>
          ) : (
            <div className="flex flex-col gap-6">
              {pages.map(([page, list]) => (
                <section key={page}>
                  <h3 className="mb-2.5 flex items-baseline gap-2 text-[14px] font-semibold">
                    Trang {page}
                    <span className="text-[13px] font-normal text-ink-3">{list.length} đoạn</span>
                  </h3>
                  <div className="flex flex-col gap-2.5">
                    {list.map((c) => (
                      <ChunkCard key={c.id} docId={id} chunk={c} technical={technical} />
                    ))}
                  </div>
                </section>
              ))}
            </div>
          )}
        </>
      )}
    </>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="flex gap-3">
      <span className="grid size-7 flex-none place-items-center rounded-full bg-blueprint-soft font-mono text-[13px] font-medium text-blueprint">
        {n}
      </span>
      <div>
        <div className="text-[14px] font-medium">{title}</div>
        <p className="mt-0.5 text-[13px] leading-relaxed text-ink-2">{children}</p>
      </div>
    </li>
  );
}

function Panel({ children, tone }: { children: React.ReactNode; tone?: "bad" }) {
  return (
    <p
      className={cn(
        "rounded-[12px] border border-rule bg-paper px-5 py-10 text-center text-sm",
        tone === "bad" ? "text-bad" : "text-ink-3",
      )}
    >
      {children}
    </p>
  );
}

function ChunkCard({ docId, chunk: c, technical }: { docId: string; chunk: Chunk; technical: boolean }) {
  const navigate = useNavigate();
  const jumpTo = useAskStore((s) => s.jumpTo);
  const [open, setOpen] = useState(false);

  // Mở màn hỏi đáp ở đúng trang và tô sáng đoạn này.
  const view = () => {
    jumpTo(docId, {
      messageId: `chunk-${c.id}`,
      ref: 0,
      page: c.page_number,
      charStart: c.char_start,
      charEnd: c.char_end,
    });
    navigate(`/documents/${docId}`);
  };

  return (
    <article className="rounded-[10px] border border-rule bg-paper px-4 py-3.5 transition-colors hover:border-ink-3/40">
      <div className="mb-1.5 flex items-center justify-between gap-3">
        <span className="text-[13px] font-medium text-ink-3">Đoạn {c.index + 1}</span>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-[13px] text-ink-2 hover:bg-field"
            onClick={() => setOpen((o) => !o)}
          >
            {open ? "Thu gọn" : "Đọc hết"}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-[13px] text-blueprint hover:bg-field"
            onClick={view}
          >
            Xem trên trang →
          </Button>
        </div>
      </div>
      <p className={cn("text-[14px] leading-relaxed text-ink", open ? "whitespace-pre-wrap" : "line-clamp-3")}>
        {c.content}
      </p>
      {technical && (
        <p className="mt-2 font-mono text-[11.5px] text-ink-3">
          mã {c.id} · ký tự {c.char_start}–{c.char_end} ({c.char_end - c.char_start}) · {c.token_count ?? "?"} token
        </p>
      )}
    </article>
  );
}
