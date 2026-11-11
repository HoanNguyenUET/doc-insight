import { useState } from "react";
import { Link, useLocation } from "react-router";

import { useOpenDocument } from "@/components/documents/use-open-document";
import { AskIcon, ChevronIcon, DocsIcon, IndexIcon, UploadIcon } from "@/components/icons";
import type { Document } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { displayName, statusTone, toneLabel } from "@/lib/format";
import { useDocuments } from "@/lib/queries";
import { cn } from "@/lib/utils";

type Item = { label: string; href: string | null; active: boolean; icon: typeof DocsIcon };

const ITEM =
  "flex w-full items-center gap-[11px] rounded-[7px] px-2.5 py-[9px] text-left text-[14.5px] text-ink-2 transition-colors";

// Thanh điều hướng bên trái. "Tài liệu" mở ra danh sách tài liệu để chuyển
// nhanh giữa các tài liệu. "Hỏi đáp" và "Nội dung" trỏ tới tài liệu đang
// mở, hoặc tài liệu của hội thoại gần nhất khi đang ở màn khác.
export function Rail() {
  const { pathname } = useLocation();
  const fromPath = pathname.match(/^\/documents\/([^/]+)/)?.[1];
  const routeDoc = fromPath && fromPath !== "upload" ? fromPath : undefined;
  const lastDoc = useAskStore((s) => s.conversations.find((c) => c.id === s.activeConversation)?.docId);
  const docId = routeDoc ?? lastDoc;
  const [listOpen, setListOpen] = useState(true);

  const items: Item[] = [
    {
      label: "Hỏi đáp",
      href: docId ? `/documents/${docId}` : null,
      active: !!routeDoc && pathname === `/documents/${routeDoc}`,
      icon: AskIcon,
    },
    { label: "Tải lên", href: "/documents/upload", active: pathname === "/documents/upload", icon: UploadIcon },
    {
      label: "Nội dung",
      href: docId ? `/documents/${docId}/chunks` : null,
      active: pathname.endsWith("/chunks"),
      icon: IndexIcon,
    },
  ];

  return (
    <aside className="flex h-screen w-[232px] flex-none flex-col border-r border-rule bg-paper">
      <div className="flex items-baseline gap-[9px] px-[22px] pt-[26px] pb-[22px]">
        <b className="text-[19px] font-semibold tracking-[-0.02em]">Doc Insight</b>
        <span className="font-mono text-[10px] tracking-[0.04em] text-ink-3">v0.4</span>
      </div>
      <nav className="flex min-h-0 flex-1 flex-col gap-px px-3 pb-4">
        <div className="flex items-center gap-0.5">
          <Link
            to="/documents"
            onClick={() => setListOpen(true)}
            className={cn(ITEM, pathname === "/documents" ? "bg-blueprint font-medium text-white" : "hover:bg-field hover:text-ink")}
            aria-current={pathname === "/documents" ? "page" : undefined}
          >
            <DocsIcon className="size-[17px] flex-none stroke-[1.6]" />
            Tài liệu
          </Link>
          <button
            type="button"
            onClick={() => setListOpen((o) => !o)}
            aria-expanded={listOpen}
            aria-label={listOpen ? "Thu gọn danh sách tài liệu" : "Mở danh sách tài liệu"}
            className="grid size-8 flex-none place-items-center rounded-[7px] text-ink-3 hover:bg-field hover:text-ink"
          >
            <ChevronIcon className={cn("size-4 stroke-[1.8] transition-transform", listOpen && "rotate-90")} />
          </button>
        </div>
        {listOpen && <DocumentList activeId={routeDoc} />}

        {items.map(({ label, href, active, icon: Ico }) => {
          const cls = cn(ITEM, active ? "bg-blueprint font-medium text-white" : "hover:bg-field hover:text-ink");
          const icon = <Ico className="size-[17px] flex-none stroke-[1.6]" />;
          return href ? (
            <Link key={label} to={href} className={cls} aria-current={active ? "page" : undefined}>
              {icon}
              {label}
            </Link>
          ) : (
            <span
              key={label}
              className={cn(cls, "cursor-not-allowed opacity-50 hover:bg-transparent hover:text-ink-2")}
              title="Mở một tài liệu trước"
              aria-disabled
            >
              {icon}
              {label}
            </span>
          );
        })}
      </nav>
    </aside>
  );
}

const DOT = { ready: "bg-ok", proc: "animate-blip bg-warn", err: "bg-bad" } as const;

// Danh sách tài liệu lồng dưới mục "Tài liệu"; dài thì cuộn trong khung.
function DocumentList({ activeId }: { activeId: string | undefined }) {
  const { data: docs, isPending, isError } = useDocuments();
  const openDocument = useOpenDocument();

  if (isPending) return <p className="py-1.5 pl-[38px] text-[13px] text-ink-3">Đang tải…</p>;
  if (isError) return <p className="py-1.5 pl-[38px] text-[13px] text-bad">Không tải được danh sách</p>;
  if (docs.length === 0)
    return (
      <Link to="/documents/upload" className="py-1.5 pl-[38px] text-[13px] text-blueprint hover:underline">
        Chưa có tài liệu, tải lên ngay
      </Link>
    );

  return (
    <ul className="mb-1.5 max-h-[45vh] overflow-y-auto border-l border-rule-soft py-0.5 pl-1.5 ml-[18px] [scrollbar-width:thin]">
      {docs.map((d) => (
        <DocumentRow key={d.id} doc={d} active={d.id === activeId} onOpen={() => openDocument(d)} />
      ))}
    </ul>
  );
}

function DocumentRow({ doc, active, onOpen }: { doc: Document; active: boolean; onOpen: () => void }) {
  const tone = statusTone(doc.status);
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        title={`${doc.filename} · ${toneLabel[tone]}`}
        aria-current={active ? "page" : undefined}
        className={cn(
          "flex w-full items-center gap-2 rounded-[6px] px-2 py-[6px] text-left text-[13.5px] text-ink-2 hover:bg-field hover:text-ink",
          active && "bg-blueprint-soft font-medium text-blueprint hover:bg-blueprint-soft hover:text-blueprint",
          tone !== "ready" && "text-ink-3",
        )}
      >
        <span className={cn("size-[7px] flex-none rounded-full", DOT[tone])} aria-hidden />
        <span className="truncate">{displayName(doc.filename)}</span>
      </button>
    </li>
  );
}
