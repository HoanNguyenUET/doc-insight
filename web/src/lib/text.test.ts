import { describe, expect, it } from "vitest";

import { foldVietnamese } from "@/lib/text";

describe("foldVietnamese", () => {
  it.each([
    { in: "Điều 208", want: "dieu 208" },
    { in: "Hồ Sơ HOÀN CÔNG", want: "ho so hoan cong" },
    { in: "Hồ sơ".normalize("NFD"), want: "ho so" },
    { in: "Certiﬁcate", want: "certificate" },
  ])("$in", ({ in: input, want }) => {
    expect(foldVietnamese(input)).toBe(want);
  });
});
