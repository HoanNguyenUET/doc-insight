"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { AskIcon, DocsIcon, IndexIcon, UploadIcon } from "@/components/icons";
import { useAskStore } from "@/lib/ask-store";
import { cn } from "@/lib/utils";

type Item = { label: string; href: string | null; active: boolean; icon: typeof DocsIcon };

// Thanh điều hướng bên trái. "Hỏi đáp" và "Chỉ mục" trỏ tới tài liệu đang
// mở, hoặc tài liệu của hội thoại gần nhất khi đang ở màn khác.
export function Rail() {
  const pathname = usePathname();
  const fromPath = pathname.match(/^\/documents\/([^/]+)/)?.[1];
  const routeDoc = fromPath && fromPath !== "upload" ? fromPath : undefined;
  const lastDoc = useAskStore((s) => s.conversations.find((c) => c.id === s.activeConversation)?.docId);
  const docId = routeDoc ?? lastDoc;

  const items: Item[] = [
    { label: "Tài liệu", href: "/documents", active: pathname === "/documents", icon: DocsIcon },
    {
      label: "Hỏi đáp",
      href: docId ? `/documents/${docId}` : null,
      active: !!routeDoc && pathname === `/documents/${routeDoc}`,
      icon: AskIcon,
    },
    { label: "Tải lên", href: "/documents/upload", active: pathname === "/documents/upload", icon: UploadIcon },
    {
      label: "Chỉ mục",
      href: docId ? `/documents/${docId}/chunks` : null,
      active: pathname.endsWith("/chunks"),
      icon: IndexIcon,
    },
  ];

  return (
    <aside className="flex h-screen w-[216px] flex-none flex-col border-r border-rule bg-paper">
      <div className="flex items-baseline gap-[9px] px-[22px] pt-[26px] pb-[22px]">
        <b className="text-[19px] font-semibold tracking-[-0.02em]">Doc Insight</b>
        <span className="font-mono text-[10px] tracking-[0.04em] text-ink-3">v0.4</span>
      </div>
      <nav className="flex flex-col gap-px px-3">
        {items.map(({ label, href, active, icon: Ico }) => {
          const cls = cn(
            "flex w-full items-center gap-[11px] rounded-[7px] px-2.5 py-[9px] text-left text-[14.5px] text-ink-2 transition-colors",
            active ? "bg-blueprint font-medium text-white" : "hover:bg-field hover:text-ink",
          );
          const icon = <Ico className="size-[17px] flex-none stroke-[1.6]" />;
          return href ? (
            <Link key={label} href={href} className={cls} aria-current={active ? "page" : undefined}>
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
