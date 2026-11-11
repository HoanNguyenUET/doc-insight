import { useEffect, useRef, useState } from "react";

import { CloseIcon, SendIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { useAskStore } from "@/lib/ask-store";
import { questionBudget } from "@/lib/question";

const DEFAULT_QUOTE_QUESTION = "Giải thích đoạn này";

// Ô nhập câu hỏi, kèm khối trích dẫn khi người dùng bôi đen ở cột trái.
export function Composer({
  disabled,
  onSend,
}: {
  disabled: boolean;
  onSend: (text: string, quote: string | null) => void;
}) {
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
    <div className="flex-none px-4 pt-2 pb-3.5">
      <form
        className="rounded-card border border-rule bg-white p-1.5 transition-colors focus-within:border-blueprint/50 focus-within:shadow-focus"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {pendingQuote && (
          <div className="m-1 mb-1.5 flex items-start justify-between gap-2.5 rounded-control border-l-[2.5px] border-marker bg-field px-3 py-2">
            <p className="line-clamp-2 font-serif text-ui leading-normal text-ink-2 italic">“{pendingQuote}”</p>
            <Button
              type="button"
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
        <div className="flex items-center gap-1.5">
          <input
            ref={input}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            maxLength={questionBudget(pendingQuote)}
            placeholder={pendingQuote ? "Hỏi gì về đoạn này? (để trống: giải thích đoạn này)" : "Hỏi về tài liệu này…"}
            aria-label="Câu hỏi"
            className="h-9 min-w-0 flex-1 bg-transparent px-2.5 text-body outline-none placeholder:text-ink-3"
          />
          <Button
            type="submit"
            size="icon"
            disabled={disabled}
            aria-label="Gửi"
            className="size-9 flex-none rounded-control"
          >
            <SendIcon className="size-[17px] stroke-[1.9]" />
          </Button>
        </div>
      </form>
      <p className="mt-1.5 px-1 text-meta text-ink-3">Enter để gửi · Bôi đen chữ trong tài liệu để hỏi về đoạn đó</p>
    </div>
  );
}
