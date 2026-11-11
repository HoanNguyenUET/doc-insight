import { useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { DropIcon } from "@/components/icons";
import { uploadDocument, type DocumentStatus } from "@/lib/api";
import { formatBytes, stageLabel } from "@/lib/format";
import { queryKeys } from "@/lib/query-client";
import { useDocument } from "@/lib/queries";
import { cn } from "@/lib/utils";

const MAX_BYTES = 50 << 20;

// Một file trong hàng đợi của phiên này. Khi đã có id thì trạng thái xử lý
// lấy từ server qua useDocument, không lưu ở đây.
type Item = { key: string; name: string; size: number; percent: number; id?: string; error?: string };

export function UploadPanel() {
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const [items, setItems] = useState<Item[]>([]);

  const patch = (key: string, p: Partial<Item>) =>
    setItems((xs) => xs.map((x) => (x.key === key ? { ...x, ...p } : x)));

  const add = (files: FileList | null) => {
    for (const file of Array.from(files ?? [])) {
      if (!/\.pdf$/i.test(file.name) && file.type !== "application/pdf") {
        toast.error(`${file.name}: chỉ nhận file PDF`);
        continue;
      }
      if (file.size > MAX_BYTES) {
        toast.error(`${file.name}: vượt quá 50 MB`);
        continue;
      }
      const key = crypto.randomUUID();
      setItems((xs) => [{ key, name: file.name, size: file.size, percent: 0 }, ...xs]);
      uploadDocument(file, (percent) => patch(key, { percent }))
        .then(({ id }) => {
          patch(key, { id, percent: 100 });
          void qc.invalidateQueries({ queryKey: queryKeys.documents });
        })
        .catch((err: Error) => patch(key, { error: err.message }));
    }
  };

  return (
    <div className="grid grid-cols-1 items-start gap-[26px] lg:grid-cols-[1fr_310px]">
      <div>
        <button
          type="button"
          onClick={() => input.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setOver(true);
          }}
          onDragLeave={() => setOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setOver(false);
            add(e.dataTransfer.files);
          }}
          className={cn(
            "block w-full cursor-pointer rounded-[10px] border-[1.5px] border-dashed border-rule bg-paper px-[30px] py-[46px] text-center transition-colors hover:border-blueprint hover:bg-white",
            over && "border-blueprint bg-white",
          )}
        >
          <DropIcon className="mx-auto mb-[13px] size-[30px] stroke-[1.4] text-blueprint" />
          <h3 className="mb-[5px] text-[17px] font-medium">Kéo thả file PDF vào đây</h3>
          <p className="text-sm text-ink-3">Hoặc bấm để chọn. Tối đa 50 MB mỗi file.</p>
        </button>
        <input
          ref={input}
          type="file"
          accept="application/pdf,.pdf"
          multiple
          hidden
          onChange={(e) => {
            add(e.target.files);
            e.target.value = "";
          }}
        />

        {items.length > 0 && (
          <Panel title="Hàng đợi" aside={`${items.length} file`} className="mt-[22px]">
            {items.map((it) => (
              <QueueRow key={it.key} item={it} />
            ))}
          </Panel>
        )}
      </div>

      <StepsPanel item={items[0]} />
    </div>
  );
}

function Panel({
  title,
  aside,
  className,
  children,
}: {
  title: string;
  aside?: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <section className={cn("overflow-hidden rounded-[10px] border border-rule bg-paper", className)}>
      <div className="flex items-center justify-between border-b border-rule-soft px-[18px] py-3.5">
        <h3 className="text-[15px] font-semibold">{title}</h3>
        {aside && <span className="text-[13px] text-ink-3">{aside}</span>}
      </div>
      {children}
    </section>
  );
}

function Spinner() {
  return <span className="inline-block size-[13px] animate-spin rounded-full border-[1.8px] border-rule border-t-warn" />;
}

function QueueRow({ item }: { item: Item }) {
  const { data: doc } = useDocument(item.id);

  let state: React.ReactNode;
  let bar: { width: number; color: string };
  if (item.error) {
    state = <span className="text-bad">{item.error}</span>;
    bar = { width: 100, color: "bg-bad" };
  } else if (!item.id || !doc) {
    state = <span className="text-blueprint">{item.percent}%</span>;
    bar = { width: item.percent, color: "bg-blueprint" };
  } else if (doc.status === "ready") {
    state = (
      <Link to={`/documents/${doc.id}`} className="text-ok hover:underline">
        Xong · {doc.chunk_count} đoạn
      </Link>
    );
    bar = { width: 100, color: "bg-ok" };
  } else if (doc.status === "failed") {
    state = <span className="text-bad" title={doc.error ?? undefined}>Lỗi: {doc.error ?? "không đọc được"}</span>;
    bar = { width: 100, color: "bg-bad" };
  } else {
    state = (
      <span className="flex items-center gap-1.5 text-warn">
        <Spinner />
        {stageLabel[doc.status]}
      </span>
    );
    bar = { width: doc.progress, color: "bg-marker" };
  }

  return (
    <div className="border-b border-rule-soft px-[18px] py-3.5 last:border-b-0">
      <div className="mb-[9px] flex items-center justify-between gap-3.5">
        <div className="min-w-0">
          <div className="truncate text-sm font-[450]">{item.name}</div>
          <div className="font-mono text-[11.5px] text-ink-3">{formatBytes(item.size)}</div>
        </div>
        <div className="max-w-[55%] truncate text-right text-[13px] whitespace-nowrap">{state}</div>
      </div>
      <div className="h-[3px] overflow-hidden rounded-sm bg-rule-soft">
        <i className={cn("block h-full rounded-sm transition-[width]", bar.color)} style={{ width: `${bar.width}%` }} />
      </div>
    </div>
  );
}

const STEPS: { title: string; desc: string; status: DocumentStatus }[] = [
  { title: "Nhận file", desc: "Kiểm tra đúng là PDF và lưu lại", status: "pending" },
  { title: "Đọc chữ", desc: "Lấy chữ của từng trang, nhớ vị trí để trích dẫn", status: "extracting" },
  { title: "Chia đoạn", desc: "Chia thành các đoạn ngắn khoảng nửa trang", status: "chunking" },
  { title: "Lập chỉ mục", desc: "Để AI tìm nhanh đoạn liên quan tới câu hỏi", status: "embedding" },
];
const ORDER: DocumentStatus[] = ["pending", "extracting", "chunking", "embedding", "ready"];

// Các bước xử lý của file vừa thêm gần nhất.
function StepsPanel({ item }: { item?: Item }) {
  const { data: doc } = useDocument(item?.id);
  // Vị trí bước đang chạy; -1 là chưa có file nào.
  let current = -1;
  if (item && !item.error) current = doc ? ORDER.indexOf(doc.status) : 0;
  const failed = !!item?.error || doc?.status === "failed";

  return (
    <Panel title="AI xử lý tài liệu thế nào">
      <ol className="py-1.5">
        {STEPS.map((s, i) => {
          const state = i < current ? "done" : i === current && !failed ? "now" : "todo";
          return (
            <li
              key={s.title}
              className="relative flex gap-[13px] px-[18px] py-[11px] before:absolute before:top-[30px] before:-bottom-[11px] before:left-[30px] before:w-px before:bg-rule last:before:hidden"
            >
              <i
                className={cn(
                  "z-[1] grid size-[25px] flex-none place-items-center rounded-full bg-field text-xs font-semibold text-ink-3 not-italic",
                  state === "done" && "bg-ok-soft text-ok",
                  state === "now" && "bg-warn-soft",
                )}
              >
                {state === "done" ? "✓" : state === "now" ? <Spinner /> : i + 1}
              </i>
              <div>
                <h4 className={cn("text-sm leading-snug font-medium", state === "todo" && "text-ink-3")}>{s.title}</h4>
                <p className="text-[13px] leading-[1.45] text-ink-3">{s.desc}</p>
              </div>
            </li>
          );
        })}
      </ol>
      {failed && (
        <p className="border-t border-rule-soft px-[18px] py-3 text-[13px] text-bad">
          {item?.error ?? doc?.error ?? "Xử lý thất bại"}
        </p>
      )}
    </Panel>
  );
}
