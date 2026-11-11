import type { Document, DocumentStatus } from "@/lib/api";

export type StateKind = "ready" | "processing" | "failed" | "stale";
export type StateTone = "ok" | "warn" | "bad";
export type DocumentState = { kind: StateKind; label: string; tone: StateTone };

// isProcessing: tài liệu chưa xử lý xong (còn đang chờ hoặc đang chạy).
export const isProcessing = (s: DocumentStatus) => s !== "ready" && s !== "failed";

// documentState là nguồn duy nhất cho trạng thái hiển thị của tài liệu
// (thanh trái, thẻ, nhãn, khung tài liệu, có mở được hay không).
export function documentState(doc: Document): DocumentState {
  if (isProcessing(doc.status)) return { kind: "processing", label: `Đang xử lý ${doc.progress}%`, tone: "warn" };
  if (doc.status === "failed") return { kind: "failed", label: "Không đọc được", tone: "bad" };
  if (doc.needs_reprocess) return { kind: "stale", label: "Cần xử lý lại", tone: "warn" };
  return { kind: "ready", label: "Sẵn sàng", tone: "ok" };
}

// Màu chữ theo tone, dùng chung cho các dòng trạng thái.
export const toneText: Record<StateTone, string> = { ok: "text-ok", warn: "text-warn", bad: "text-bad" };
