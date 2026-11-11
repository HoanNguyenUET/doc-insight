import type { Document, DocumentStatus } from "@/lib/api";

export type StatusTone = "ready" | "proc" | "err";

export function statusTone(s: DocumentStatus): StatusTone {
  if (s === "ready") return "ready";
  if (s === "failed") return "err";
  return "proc";
}

export const toneLabel: Record<StatusTone, string> = {
  ready: "Sẵn sàng",
  proc: "Đang xử lý",
  err: "Không đọc được",
};

export const stageLabel: Record<DocumentStatus, string> = {
  pending: "Đang chờ",
  extracting: "Đang đọc chữ",
  chunking: "Đang chia đoạn",
  embedding: "Đang lập chỉ mục",
  ready: "Xong",
  failed: "Lỗi",
};

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

const dateFmt = new Intl.DateTimeFormat("vi-VN", { day: "2-digit", month: "2-digit", year: "numeric", timeZone: "Asia/Ho_Chi_Minh" });

export const formatDate = (iso: string) => dateFmt.format(new Date(iso));

// Thời gian worker xử lý, không tính lúc chờ trong hàng đợi.
export function processingSeconds(d: Document): number | null {
  if (!d.processing_started_at || !d.processed_at) return null;
  const ms = new Date(d.processed_at).getTime() - new Date(d.processing_started_at).getTime();
  return Math.max(0, ms / 1000);
}

export const displayName = (filename: string) => filename.replace(/\.pdf$/i, "");
