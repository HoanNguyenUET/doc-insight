"use client";

import { useEffect, useRef, useState } from "react";

import { CloseIcon, SendIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAskStore } from "@/lib/ask-store";

const MAX_QUESTION_CHARS = 300;
const DEFAULT_QUOTE_QUESTION = "Giải thích đoạn này";

// Ô nhập câu hỏi, kèm khối trích dẫn khi người dùng bôi đen ở cột trái.
export function Composer({ disabled, onSend }: { disabled: boolean; onSend: (text: string, quote: string | null) => void }) {
  const pendingQuote = useAskStore((s) => s.pendingQuote);
  const setPendingQuote = useAskStore((s) => s.setPendingQuote);
  const [value, setValue] = useState("");
  const input = useRef<HTMLInputElement>(null);

  // Vừa chèn trích dẫn thì đưa con trỏ vào ô hỏi.
  useEffect(() => {
    if (pendingQuote) input.current?.focus();
  }, [pendingQuote]);

  const submit = () => {
    const text = value.trim() || (pendingQuote ? DEFAULT_QUOTE_QUESTION : "");
    if (!text || disabled) return;
    onSend(text, pendingQuote);
    setValue("");
  };

  return (
    <div className="border-t border-rule px-4 py-3.5">
      {pendingQuote && (
        <div className="mb-2.5 flex items-start justify-between gap-2.5 rounded-r-[7px] border-l-[2.5px] border-marker bg-field px-3 py-[9px]">
          <p className="line-clamp-2 font-serif text-[13.5px] leading-normal text-ink-2 italic">“{pendingQuote}”</p>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => setPendingQuote(null)}
            aria-label="Bỏ trích dẫn"
            className="size-5 flex-none text-ink-3 hover:bg-transparent hover:text-ink"
          >
            <CloseIcon className="size-[15px] stroke-[1.8]" />
          </Button>
        </div>
      )}
      <form
        className="flex items-end gap-[9px]"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Input
          ref={input}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          maxLength={MAX_QUESTION_CHARS}
          placeholder={pendingQuote ? "Hỏi gì về đoạn này? (để trống: giải thích đoạn này)" : "Hỏi về tài liệu này"}
          aria-label="Câu hỏi"
          className="h-10 flex-1 rounded-[9px] border-transparent bg-field px-3.5 text-[14.5px] focus-visible:border-blueprint focus-visible:bg-white"
        />
        <Button type="submit" size="icon" disabled={disabled} aria-label="Gửi" className="size-10 flex-none rounded-[9px]">
          <SendIcon className="size-[17px] stroke-[1.9]" />
        </Button>
      </form>
    </div>
  );
}
