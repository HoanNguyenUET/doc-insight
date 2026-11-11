import { describe, expect, it } from "vitest";

import { parseAnswer, splitOutside, type Part } from "@/lib/parse-answer";

const t = (value: string): Part => ({ kind: "text", value });
const c = (n: number): Part => ({ kind: "cite", n });

describe("parseAnswer", () => {
  it.each([
    {
      name: "chữ và trích dẫn",
      text: "Nghỉ 12 ngày [[1]].",
      streaming: false,
      want: [[t("Nghỉ 12 ngày "), c(1), t(".")]],
    },
    { name: "nhiều trích dẫn liền nhau", text: "A [[1]][[3]]", streaming: false, want: [[t("A "), c(1), c(3)]] },
    {
      name: "tách đoạn theo dòng trống",
      text: "Một.\n\n\nHai [[2]]",
      streaming: false,
      want: [[t("Một.")], [t("Hai "), c(2)]],
    },
    { name: "một lần xuống dòng không tách đoạn", text: "Một\nhai", streaming: false, want: [[t("Một\nhai")]] },
    {
      name: "đang stream: bỏ trích dẫn viết dở",
      text: "Câu trả lời [[1",
      streaming: true,
      want: [[t("Câu trả lời ")]],
    },
    { name: "xong rồi thì giữ nguyên chữ", text: "Giá [[1", streaming: false, want: [[t("Giá [[1")]] },
    { name: "rỗng", text: "", streaming: false, want: [] },
  ])("$name", ({ text, streaming, want }) => {
    expect(parseAnswer(text, streaming)).toEqual(want);
  });
});

describe("splitOutside", () => {
  it.each([
    { name: "không có nhãn", text: "Từ tài liệu [[1]].", want: [[[t("Từ tài liệu "), c(1), t(".")]], []] },
    {
      name: "nhãn cùng dòng với nội dung",
      text: "Tài liệu không đề cập.\n\nNgoài tài liệu: Paris.",
      want: [[[t("Tài liệu không đề cập.")]], [[t("Paris.")]]],
    },
    {
      name: "nhãn rồi xuống dòng, các đoạn sau cũng là ngoài tài liệu",
      text: "A [[1]]\n\nNgoài tài liệu:\nB.\n\nC.",
      want: [[[t("A "), c(1)]], [[t("B.")], [t("C.")]]],
    },
    { name: "nhãn đứng riêng một đoạn", text: "Ngoài tài liệu:\n\nB.", want: [[], [[t("B.")]]] },
  ])("$name", ({ text, want }) => {
    expect(splitOutside(parseAnswer(text, false))).toEqual(want);
  });
});
