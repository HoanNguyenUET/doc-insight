import { describe, expect, it } from "vitest";

import { MAX_QUESTION_CHARS, buildQuestion, questionBudget } from "@/lib/question";

describe("buildQuestion", () => {
  it.each([
    { name: "không trích đoạn", text: "Hỏi gì?", quote: null, want: "Hỏi gì?" },
    { name: "kèm trích đoạn", text: "Giải thích", quote: "Điều 5", want: "Về đoạn trích «Điều 5»: Giải thích" },
  ])("$name", ({ text, quote, want }) => {
    expect(buildQuestion(text, quote)).toBe(want);
  });
});

// Ô nhập không được cho gõ quá giới hạn của API (1000 ký tự) sau khi đã cộng
// đoạn trích và phần bọc.
describe("questionBudget", () => {
  it.each([
    { name: "không trích đoạn", quote: null },
    { name: "trích đoạn ngắn", quote: "Điều 208. Mục kiểm tra" },
    { name: "trích đoạn dài", quote: "x".repeat(600) },
  ])("$name", ({ quote }) => {
    const budget = questionBudget(quote);
    expect(budget).toBeGreaterThan(0);
    expect(buildQuestion("a".repeat(budget), quote).length).toBe(MAX_QUESTION_CHARS);
  });
});
