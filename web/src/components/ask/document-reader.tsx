import "react-pdf/dist/Page/AnnotationLayer.css";
import "react-pdf/dist/Page/TextLayer.css";

import { useEffect, useRef, useState } from "react";
import { Document as PdfDocument, Page as PdfPage, pdfjs } from "react-pdf";

import { ReaderToolbar } from "@/components/ask/reader-toolbar";
import { SelectionQuote } from "@/components/ask/selection-quote";
import { usePdfHighlight } from "@/components/ask/use-pdf-highlight";
import { useElementWidth } from "@/hooks/use-element-width";
import { api, type Document } from "@/lib/api";
import { useAskStore } from "@/lib/ask-store";

// react-pdf yêu cầu đặt worker ngay trong module dùng <Document>/<Page>.
pdfjs.GlobalWorkerOptions.workerSrc = new URL("pdfjs-dist/build/pdf.worker.min.mjs", import.meta.url).toString();

const MAX_PAGE_WIDTH = 900;
// Mức phóng; 1 = vừa khít bề ngang cột.
const ZOOMS = [1, 1.25, 1.5, 2, 2.5];

// Cột trái: trang PDF thật (pdf.js), tô sáng đoạn nguồn của trích dẫn đang
// chọn, và nút "Hỏi về đoạn này" khi bôi đen.
export function DocumentReader({ doc }: { doc: Document }) {
  const currentPage = useAskStore((s) => s.currentPage);
  const setCurrentPage = useAskStore((s) => s.setCurrentPage);

  const scrollRef = useRef<HTMLDivElement>(null);
  const sheetRef = useRef<HTMLDivElement>(null);
  const width = useElementWidth(scrollRef);
  const [zoom, setZoom] = useState(0);
  const { renderText, onGetText, onTextRendered, notFound } = usePdfHighlight(doc.id, currentPage, sheetRef);

  const pageCount = doc.page_count ?? 1;
  const pageWidth = Math.round(Math.min(width - 40, MAX_PAGE_WIDTH) * ZOOMS[zoom]);

  // Lật trang bằng tay thì về đầu trang.
  useEffect(() => {
    if (!useAskStore.getState().activeCitation) scrollRef.current?.scrollTo({ top: 0 });
  }, [currentPage]);

  const turn = (delta: number) => {
    const next = Math.min(pageCount, Math.max(1, currentPage + delta));
    if (next !== currentPage) setCurrentPage(next);
  };

  return (
    <section className="flex min-w-0 flex-[1_1_56%] flex-col border-r border-rule bg-field">
      <ReaderToolbar
        filename={doc.filename}
        zoomPercent={ZOOMS[zoom] * 100}
        canZoomOut={zoom > 0}
        canZoomIn={zoom < ZOOMS.length - 1}
        onZoom={(d) => setZoom((z) => z + d)}
        page={currentPage}
        pageCount={pageCount}
        onTurn={turn}
      />

      <div ref={scrollRef} className="relative flex-1 overflow-auto px-5 pt-7 pb-10">
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

        <SelectionQuote scrollRef={scrollRef} sheetRef={sheetRef} page={currentPage} />
      </div>
    </section>
  );
}

function Notice({ children }: { children: React.ReactNode }) {
  return <p className="w-[min(620px,80vw)] px-10 py-24 text-center text-sm text-ink-3">{children}</p>;
}
