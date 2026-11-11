import { Button } from "@/components/ui/button";

// ReaderToolbar: phóng to/thu nhỏ và lật trang của cột tài liệu (tên tài
// liệu đã ở thanh tiêu đề phía trên).
export function ReaderToolbar({
  zoomPercent,
  canZoomOut,
  canZoomIn,
  onZoom,
  page,
  pageCount,
  onTurn,
}: {
  zoomPercent: number;
  canZoomOut: boolean;
  canZoomIn: boolean;
  onZoom: (delta: number) => void;
  page: number;
  pageCount: number;
  onTurn: (delta: number) => void;
}) {
  return (
    <div className="flex h-[52px] flex-none items-center justify-end gap-3 border-b border-rule-soft bg-paper px-4">
      <div className="flex flex-none items-center gap-px rounded-[7px] bg-field p-0.5">
        <ToolButton onClick={() => onZoom(-1)} disabled={!canZoomOut} label="Thu nhỏ">
          −
        </ToolButton>
        <span className="w-11 text-center font-mono text-xs text-ink-2">{zoomPercent}%</span>
        <ToolButton onClick={() => onZoom(1)} disabled={!canZoomIn} label="Phóng to">
          +
        </ToolButton>
      </div>
      <div className="flex flex-none items-center gap-px rounded-[7px] bg-field p-0.5">
        <ToolButton onClick={() => onTurn(-1)} disabled={page <= 1} label="Trang trước">
          ‹
        </ToolButton>
        <span className="px-2 font-mono text-xs whitespace-nowrap text-ink-2">
          <b>{page}</b> / {pageCount}
        </span>
        <ToolButton onClick={() => onTurn(1)} disabled={page >= pageCount} label="Trang sau">
          ›
        </ToolButton>
      </div>
    </div>
  );
}

function ToolButton({
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
