import { Link } from "react-router";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// EmptyState: thông báo khi chưa có gì để hiện (đang tải, trống, lỗi, cần thao
// tác), kèm nút hành động và nút về danh sách nếu cần. framed = khung thẻ.
export function EmptyState({
  children,
  action,
  back,
  tone = "muted",
  framed,
  className,
}: {
  children: React.ReactNode;
  action?: React.ReactNode;
  back?: boolean;
  tone?: "muted" | "bad";
  framed?: boolean;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "text-center text-sm leading-relaxed",
        tone === "bad" ? "text-bad" : "text-ink-3",
        framed ? "rounded-card border border-dashed border-rule bg-paper px-6 py-12" : "m-auto max-w-[46ch] px-5",
        className,
      )}
    >
      <div>{children}</div>
      {(action || back) && (
        <div className="mt-4 flex justify-center gap-2">
          {action}
          {back && (
            <Button asChild variant="outline">
              <Link to="/documents">Về danh sách tài liệu</Link>
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
