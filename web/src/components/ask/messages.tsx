import { useEffect, useState } from "react";

import { AnswerText } from "@/components/ask/answer-text";
import { SparkIcon } from "@/components/icons";
import type { StreamStatus } from "@/hooks/use-answer-stream";
import type { Message } from "@/lib/ask-store";

// Các loại tin trong khung chat: câu hỏi của người dùng, câu trả lời đã xong
// và ô "đang nghĩ" khi AI chưa viết chữ nào.

export function UserBubble({ message }: { message: Extract<Message, { role: "user" }> }) {
  return (
    <div className="max-w-[84%] self-end rounded-[14px_14px_4px_14px] bg-blueprint px-[15px] py-[11px] text-[14.5px] leading-[1.55] whitespace-pre-wrap text-white">
      {message.quote && (
        <div className="mb-2 border-l-2 border-white/45 pl-[9px] font-serif text-[13.5px] leading-normal italic opacity-85">
          {message.quote}
        </div>
      )}
      {message.text}
    </div>
  );
}

export function AssistantMessage({ message }: { message: Extract<Message, { role: "assistant" }> }) {
  return (
    <div className="max-w-[93%] self-start text-[14.5px] leading-[1.68]">
      <AnswerText messageId={message.id} text={message.text} sources={message.sources} />
      {message.error && <p className="mt-2 text-[13px] text-bad">{message.error}</p>}
    </div>
  );
}

// Thinking hiện trong lúc AI chưa viết chữ nào: đang tìm đoạn liên quan,
// rồi đang soạn câu trả lời (model "suy nghĩ" vài giây trước token đầu).
export function Thinking({ status, found }: { status: StreamStatus; found: number }) {
  const [seconds, setSeconds] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setSeconds((s) => s + 1), 1000);
    return () => clearInterval(t);
  }, []);

  const label =
    status === "connecting"
      ? "Đang tìm các đoạn liên quan trong tài liệu"
      : `Đã tìm thấy ${found} đoạn liên quan, đang soạn câu trả lời`;
  return (
    <div
      role="status"
      className="flex max-w-[93%] items-center gap-3 self-start rounded-[12px] border border-rule-soft bg-field px-3.5 py-3 text-[13.5px] text-ink-2"
    >
      <span className="grid size-7 flex-none place-items-center rounded-full bg-blueprint-soft text-blueprint">
        <SparkIcon className="size-4 animate-pulse" />
      </span>
      <span>
        {label}
        <span className="ml-1 inline-flex gap-[3px] align-middle" aria-hidden>
          {[0, 200, 400].map((d) => (
            <span key={d} className="size-[5px] animate-blip rounded-full bg-blueprint" style={{ animationDelay: `${d}ms` }} />
          ))}
        </span>
        {seconds >= 6 && <span className="mt-0.5 block text-xs text-ink-3">{seconds} giây · câu hỏi khó có thể mất lâu hơn</span>}
      </span>
    </div>
  );
}
