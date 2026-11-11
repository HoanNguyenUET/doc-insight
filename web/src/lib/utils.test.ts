import { describe, expect, it } from "vitest";

import { cn } from "@/lib/utils";

// Token cỡ chữ, bán kính, bóng tự đặt tên (globals.css) phải được cn() hiểu
// đúng nhóm, nếu không twMerge coi text-body là màu và bỏ mất text-ink.
describe("cn với token thiết kế", () => {
  it.each([
    { name: "cỡ chữ và màu cùng tồn tại", in: ["text-body text-ink"], want: "text-body text-ink" },
    { name: "hai cỡ chữ: giữ cái sau", in: ["text-small", "text-body"], want: "text-body" },
    { name: "hai bán kính: giữ cái sau", in: ["rounded-control", "rounded-card"], want: "rounded-card" },
    { name: "hai bóng: giữ cái sau", in: ["shadow-card", "shadow-sheet"], want: "shadow-sheet" },
    { name: "bóng và màu chữ cùng tồn tại", in: ["shadow-card text-blueprint"], want: "shadow-card text-blueprint" },
  ])("$name", ({ in: input, want }) => {
    expect(cn(...input)).toBe(want);
  });
});
