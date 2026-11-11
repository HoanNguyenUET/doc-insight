import { describe, expect, it } from "vitest";

import type { Document } from "@/lib/api";
import { documentState, isProcessing } from "@/lib/document-state";

const doc = (p: Partial<Document>): Document => ({
  id: "1",
  filename: "a.pdf",
  size_bytes: 1,
  page_count: 3,
  chunk_count: 3,
  status: "ready",
  progress: 100,
  error: null,
  embed_model: "m",
  needs_reprocess: false,
  processing_started_at: null,
  created_at: "",
  updated_at: "",
  processed_at: null,
  ...p,
});

describe("documentState", () => {
  it.each([
    { name: "sẵn sàng", d: doc({}), kind: "ready", label: "Sẵn sàng", tone: "ok" },
    {
      name: "đang xử lý",
      d: doc({ status: "embedding", progress: 45 }),
      kind: "processing",
      label: "Đang xử lý 45%",
      tone: "warn",
    },
    {
      name: "đang chờ",
      d: doc({ status: "pending", progress: 0 }),
      kind: "processing",
      label: "Đang xử lý 0%",
      tone: "warn",
    },
    { name: "lỗi", d: doc({ status: "failed" }), kind: "failed", label: "Không đọc được", tone: "bad" },
    { name: "cần xử lý lại", d: doc({ needs_reprocess: true }), kind: "stale", label: "Cần xử lý lại", tone: "warn" },
  ])("$name", ({ d, kind, label, tone }) => {
    expect(documentState(d)).toEqual({ kind, label, tone });
  });

  it("isProcessing", () => {
    expect(["pending", "extracting", "chunking", "embedding"].every((s) => isProcessing(s as Document["status"]))).toBe(
      true,
    );
    expect(isProcessing("ready") || isProcessing("failed")).toBe(false);
  });
});
