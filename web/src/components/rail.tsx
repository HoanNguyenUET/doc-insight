import { useState } from "react";
import { Link, useLocation } from "react-router";

import { useOpenDocument } from "@/components/documents/use-open-document";
import { DocsIcon, FileIcon, PlusIcon, SearchIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import type { Document } from "@/lib/api";
import { documentState, toneText } from "@/lib/document-state";
import { displayName } from "@/lib/format";
import { useDocuments } from "@/lib/queries";
import { cn } from "@/lib/utils";

// Thanh bên trái: tải lên, tìm và chuyển nhanh giữa các tài liệu. Hỏi đáp và
// Nội dung là tab trong màn của từng tài liệu, không nằm ở đây.
export function Rail() {
  const { pathname } = useLocation();
  const activeId = pathname.match(/^\/documents\/([^/]+)/)?.[1];
  const [query, setQuery] = useState("");

  return (
    <aside className="flex h-screen w-[264px] flex-none flex-col border-r border-rule bg-paper">
      <div className="flex items-baseline gap-2 px-5 pt-6 pb-5">
        <Link to="/documents" className="text-heading font-semibold tracking-[-0.02em]">
          Doc Insight
        </Link>
        <span className="font-mono text-2xs text-ink-3">v{__APP_VERSION__}</span>
      </div>

      <div className="flex flex-col gap-2.5 px-3">
        <Button asChild className="h-10 justify-start gap-2 rounded-control px-3.5">
          <Link to="/documents/upload">
            <PlusIcon className="size-4 stroke-2" />
            Tải tài liệu lên
          </Link>
        </Button>
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-[15px] -translate-y-1/2 stroke-[1.8] text-ink-3" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Tìm tài liệu"
            aria-label="Tìm tài liệu"
            className="h-9 w-full rounded-control border border-transparent bg-field pr-3 pl-9 text-ui outline-none placeholder:text-ink-3 focus:border-rule focus:bg-white"
          />
        </div>
        <Link
          to="/documents"
          aria-current={pathname === "/documents" ? "page" : undefined}
          className={cn(
            "flex items-center gap-2.5 rounded-control px-3 py-2 text-ui text-ink-2 transition-colors hover:bg-field hover:text-ink",
            pathname === "/documents" && "bg-field font-medium text-ink",
          )}
        >
          <DocsIcon className="size-4 stroke-[1.7]" />
          Tất cả tài liệu
        </Link>
      </div>

      <DocumentList activeId={activeId} query={query} />
    </aside>
  );
}

function DocumentList({ activeId, query }: { activeId: string | undefined; query: string }) {
  const { data: docs, isPending, isError } = useDocuments();
  const openDocument = useOpenDocument();
  const q = query.trim().toLowerCase();
  const shown = docs?.filter((d) => d.filename.toLowerCase().includes(q));

  return (
    <div className="mt-5 flex min-h-0 flex-1 flex-col">
      <div className="px-5 pb-1.5 text-2xs font-medium tracking-[0.06em] text-ink-3 uppercase">
        Tài liệu {docs && `· ${docs.length}`}
      </div>
      <div className="min-h-0 flex-1 [scrollbar-width:thin] overflow-y-auto px-3 pb-4">
        {isPending ? (
          <p className="px-3 py-2 text-small text-ink-3">Đang tải…</p>
        ) : isError ? (
          <p className="px-3 py-2 text-small text-bad">Không tải được danh sách</p>
        ) : shown!.length === 0 ? (
          <p className="px-3 py-2 text-small text-ink-3">{q ? "Không có tài liệu khớp." : "Chưa có tài liệu nào."}</p>
        ) : (
          <ul className="flex flex-col gap-px">
            {shown!.map((d) => (
              <DocumentRow key={d.id} doc={d} active={d.id === activeId} onOpen={() => openDocument(d)} />
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

// Dòng phụ dưới tên: số trang khi sẵn sàng, còn lại là trạng thái cần chú ý.
function rowMeta(doc: Document): { text: string; className: string } {
  const state = documentState(doc);
  if (state.kind !== "ready") return { text: state.label, className: toneText[state.tone] };
  return { text: doc.page_count != null ? `${doc.page_count} trang` : state.label, className: "text-ink-3" };
}

function DocumentRow({ doc, active, onOpen }: { doc: Document; active: boolean; onOpen: () => void }) {
  const meta = rowMeta(doc);
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        title={doc.filename}
        aria-current={active ? "page" : undefined}
        className={cn(
          "relative flex w-full items-center gap-2.5 rounded-control px-3 py-2 text-left transition-colors hover:bg-field",
          active && "bg-blueprint-soft hover:bg-blueprint-soft",
        )}
      >
        {active && <span className="absolute top-2 bottom-2 left-0 w-[3px] rounded-full bg-blueprint" aria-hidden />}
        <FileIcon className={cn("size-[18px] flex-none stroke-[1.5]", active ? "text-blueprint" : "text-ink-3")} />
        <span className="min-w-0">
          <span className={cn("block truncate text-ui", active ? "font-medium text-blueprint" : "text-ink")}>
            {displayName(doc.filename)}
          </span>
          <span className={cn("block text-meta", meta.className)}>{meta.text}</span>
        </span>
      </button>
    </li>
  );
}
