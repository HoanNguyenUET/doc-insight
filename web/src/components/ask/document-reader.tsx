import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Document as PdfDocument, Page as PdfPage, pdfjs } from "react-pdf";

import { QuoteAskIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { api, type Document } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";
import { locate, markItem } from "@/lib/pdf-highlight";
import { usePage } from "@/lib/queries";

// react-pdf yêu cầu đặt worker ngay trong module dùng <Document>/<Page>.
pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();

const MIN_PICK_CHARS = 12;
// Câu hỏi gửi lên tối đa 1000 ký tự; chừa chỗ cho phần người dùng gõ.
const MAX_QUOTE_CHARS = 600;
const MAX_PAGE_WIDTH = 900;
// Mức phóng; 1 = vừa khít bề ngang cột.
const ZOOMS = [1, 1.25, 1.5, 2, 2.5];
// Nền trong suốt để chữ của trang (vẽ trên canvas bên dưới) vẫn hiện rõ.
const MARK_CLASS = "animate-flash-overlay rounded-[2px] bg-marker/20 text-transparent";

type Pick = { page: number; top: number; left: number; text: string };
type PageItems = { page: number; strs: string[] };

// Cột trái: trang PDF thật (pdf.js), tô sáng đoạn nguồn của trích dẫn đang
// chọn trên text layer, và nút "Hỏi về đoạn này" khi bôi đen.
export function DocumentReader({ doc }: { doc: Document }) {
  const currentPage = useAskStore((s) => s.currentPage);
  const setCurrentPage = useAskStore((s) => s.setCurrentPage);
  const activeCitation = useAskStore((s) => s.activeCitation);
  const setPendingQuote = useAskStore((s) => s.setPendingQuote);
  // Văn bản trang do API trích: offset của chunk tính trên văn bản này.
  const { data: pageData } = usePage(doc.id, currentPage, true);

  const scrollRef = useRef<HTMLDivElement>(null);
  const sheetRef = useRef<HTMLDivElement>(null);
  const width = useWidth(scrollRef);
  const [items, setItems] = useState<PageItems | null>(null);
  const [rendered, setRendered] = useState(0);
  const [pick, setPick] = useState<Pick | null>(null);

  const pageCount = doc.page_count ?? 1;
  const [zoom, setZoom] = useState(0);
  const pageWidth = Math.round(Math.min(width - 40, MAX_PAGE_WIDTH) * ZOOMS[zoom]);

  // Đoạn nguồn chỉ có khi văn bản đang có đúng là trang của trích dẫn
  // (keepPreviousData giữ trang cũ trong lúc tải trang mới).
  const passage =
    activeCitation && activeCitation.page === currentPage && pageData?.page_number === currentPage
      ? Array.from(pageData.text).slice(activeCitation.charStart, activeCitation.charEnd).join("")
      : null;
  const ranges = useMemo(
    () => (passage && items?.page === currentPage ? locate(items.strs, passage) : null),
    [passage, items, currentPage],
  );
  const notFound = passage !== null && items?.page === currentPage && ranges === null;

  // nonce đổi khi bấm lại cùng trích dẫn: text layer vẽ lại nên animation chạy lại.
  const nonce = activeCitation?.nonce ?? 0;
  const renderText = useCallback(
    ({ itemIndex, str }: { itemIndex: number; str: string }) => markItem(str, ranges?.get(itemIndex), MARK_CLASS, nonce),
    [ranges, nonce],
  );

  // Callback phải cố định: react-pdf vẽ lại text layer mỗi khi chúng đổi,
  // và vẽ lại lại gọi chúng, thành vòng lặp.
  const onGetText = useCallback(
    (tc: { items: ({ str: string } | object)[] }) =>
      setItems({ page: currentPage, strs: tc.items.map((it) => ("str" in it ? it.str : "")) }),
    [currentPage],
  );
  const onTextRendered = useCallback(() => setRendered((n) => n + 1), []);

  // Text layer vẽ xong (rendered tăng) mới có <mark> trong DOM để cuộn tới.
  useEffect(() => {
    if (!ranges) return;
    sheetRef.current?.querySelector("mark[data-cite]")?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [ranges, rendered]);

  // Lật trang bằng tay thì về đầu trang. Nút trích dẫn gắn với trang nơi bôi
  // chọn nên tự ẩn khi sang trang khác.
  useEffect(() => {
    if (!useAskStore.getState().activeCitation) scrollRef.current?.scrollTo({ top: 0 });
  }, [currentPage]);
  const shownPick = pick && pick.page === currentPage ? pick : null;

  const turn = (delta: number) => {
    const next = Math.min(pageCount, Math.max(1, currentPage + delta));
    if (next !== currentPage) setCurrentPage(next);
  };

  const onMouseUp = () => {
    const sel = window.getSelection();
    const text = sel?.toString().trim() ?? "";
    const scroll = scrollRef.current;
    if (!sel || sel.rangeCount === 0 || text.length <= MIN_PICK_CHARS || !scroll || !sheetRef.current?.contains(sel.anchorNode)) {
      setPick(null);
      return;
    }
    const r = sel.getRangeAt(0).getBoundingClientRect();
    const box = scroll.getBoundingClientRect();
    setPick({ page: currentPage, top: r.top - box.top + scroll.scrollTop, left: r.left - box.left + r.width / 2, text });
  };

  const quote = () => {
    if (!shownPick) return;
    const text = shownPick.text.replace(/\s+/g, " ");
    setPendingQuote(text.length > MAX_QUOTE_CHARS ? text.slice(0, MAX_QUOTE_CHARS).trimEnd() + "…" : text);
    setPick(null);
    window.getSelection()?.removeAllRanges();
  };

  return (
    <section className="flex min-w-0 flex-[1_1_56%] flex-col border-r border-rule bg-field">
      <div className="flex h-[50px] flex-none items-center justify-between gap-3.5 border-b border-rule bg-paper px-4">
        <div className="truncate text-sm font-medium">{doc.filename}</div>
        <div className="ml-auto flex items-center gap-px rounded-[7px] bg-field p-0.5">
          <PagerButton onClick={() => setZoom((z) => z - 1)} disabled={zoom === 0} label="Thu nhỏ">
            −
          </PagerButton>
          <span className="w-11 text-center font-mono text-xs text-ink-2">{ZOOMS[zoom] * 100}%</span>
          <PagerButton onClick={() => setZoom((z) => z + 1)} disabled={zoom === ZOOMS.length - 1} label="Phóng to">
            +
          </PagerButton>
        </div>
        <div className="flex items-center gap-px rounded-[7px] bg-field p-0.5">
          <PagerButton onClick={() => turn(-1)} disabled={currentPage <= 1} label="Trang trước">
            ‹
          </PagerButton>
          <span className="px-2 font-mono text-xs text-ink-2">
            <b>{currentPage}</b> / {pageCount}
          </span>
          <PagerButton onClick={() => turn(1)} disabled={currentPage >= pageCount} label="Trang sau">
            ›
          </PagerButton>
        </div>
      </div>

      <div
        ref={scrollRef}
        onMouseUp={onMouseUp}
        className="relative flex-1 overflow-auto px-5 pt-7 pb-10"
      >
        <div
          ref={sheetRef}
          // mx-auto + w-fit: căn giữa khi vừa cột, cuộn ngang được khi phóng to.
          className="relative mx-auto w-fit bg-white shadow-[0_1px_2px_rgba(26,28,31,.06),0_8px_26px_rgba(26,28,31,.07)]"
        >
          <PdfDocument
            file={api.fileUrl(doc.id)}
            externalLinkTarget="_blank"
            loading={<Notice>Đang tải PDF…</Notice>}
            error={<Notice>Không mở được file PDF gốc. Có thể file đã mất trên máy chủ; hãy tải lên lại.</Notice>}
          >
            {pageWidth > 0 && (
              <PdfPage
                pageNumber={currentPage}
                width={pageWidth}
                customTextRenderer={renderText}
                onGetTextSuccess={onGetText}
                onRenderTextLayerSuccess={onTextRendered}
                loading={<Notice>Đang vẽ trang {currentPage}…</Notice>}
              />
            )}
          </PdfDocument>
        </div>

        {notFound && (
          <p className="absolute top-3 left-1/2 z-30 -translate-x-1/2 rounded-md bg-ink px-3 py-1.5 text-xs text-white shadow">
            Không định vị được đoạn nguồn trên trang {currentPage}.
          </p>
        )}

        {shownPick && (
          <Button
            size="sm"
            className="absolute z-40 h-auto -translate-x-1/2 -translate-y-[130%] gap-[7px] rounded-[7px] bg-ink px-3 py-1.5 text-[13px] font-[450] text-white shadow-[0_4px_14px_rgba(0,0,0,.2)] hover:bg-ink/90"
            style={{ top: shownPick.top, left: shownPick.left }}
            // Giữ vùng chọn khi bấm nút.
            onMouseDown={(e) => e.preventDefault()}
            onMouseUp={(e) => e.stopPropagation()}
            onClick={quote}
          >
            <QuoteAskIcon className="size-3.5 stroke-[1.8]" />
            Hỏi về đoạn này
          </Button>
        )}
      </div>
    </section>
  );
}

function PagerButton({
  children,
  label,
  ...props
}: { children: React.ReactNode; label: string } & React.ComponentProps<"button">) {
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={label}
      className="h-6 w-[26px] rounded-[5px] text-ink-2 hover:bg-paper disabled:opacity-30"
      {...props}
    >
      {children}
    </Button>
  );
}

function Notice({ children }: { children: React.ReactNode }) {
  return <p className="w-[min(620px,80vw)] px-10 py-24 text-center text-sm text-ink-3">{children}</p>;
}

// useWidth theo dõi bề rộng phần tử để vẽ trang PDF vừa cột.
function useWidth(ref: React.RefObject<HTMLElement | null>) {
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, [ref]);
  return width;
}
