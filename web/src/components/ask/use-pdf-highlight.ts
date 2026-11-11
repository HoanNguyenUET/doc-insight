import { useCallback, useEffect, useMemo, useState } from "react";

import { useAskStore } from "@/lib/ask-store";
import { locate, markItem } from "@/lib/pdf-highlight";
import { usePage } from "@/lib/queries";

// Nền trong suốt để chữ của trang (vẽ trên canvas bên dưới) vẫn hiện rõ.
const MARK_CLASS = "animate-flash-overlay rounded-[2px] bg-marker/20 text-transparent";

type PageItems = { page: number; strs: string[] };

// usePdfHighlight tô đoạn nguồn của trích dẫn đang chọn trên text layer của
// trang PDF đang hiện, rồi cuộn tới đó. Trả các callback để gắn vào <Page>.
export function usePdfHighlight(docId: string, currentPage: number, sheetRef: React.RefObject<HTMLElement | null>) {
  const activeCitation = useAskStore((s) => s.activeCitation);
  // Văn bản trang do API trích: offset của chunk tính trên văn bản này.
  const { data: pageData } = usePage(docId, currentPage, true);
  const [items, setItems] = useState<PageItems | null>(null);
  const [rendered, setRendered] = useState(0);

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
    ({ itemIndex, str }: { itemIndex: number; str: string }) =>
      markItem(str, ranges?.get(itemIndex), MARK_CLASS, nonce),
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
  }, [ranges, rendered, sheetRef]);

  return { renderText, onGetText, onTextRendered, notFound };
}
