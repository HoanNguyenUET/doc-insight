import { AnswerText } from "@/components/ask/answer-text";
import { SparkIcon } from "@/components/icons";
import type { Message } from "@/lib/ask-store";

// Các loại tin trong khung chat: câu hỏi của người dùng, câu trả lời đã xong
// và ô "đang nghĩ" khi AI chưa viết chữ nào.

export function UserBubble({ message }: { message: Extract<Message, { role: "user" }> }) {
  return (
    <div className="max-w-[84%] self-end rounded-[16px_16px_4px_16px] bg-blueprint-soft px-4 py-2.5 text-[14.5px] leading-[1.55] whitespace-pre-wrap text-ink">
      {message.quote && (
        <div className="mb-1.5 border-l-2 border-blueprint/30 pl-2.5 font-serif text-[13.5px] leading-normal text-ink-2 italic">
          {message.quote}
        </div>
      )}
      {message.text}
    </div>
  );
}

export function AssistantMessage({ message }: { message: Extract<Message, { role: "assistant" }> }) {
  return (
    <div className="max-w-[93%] self-start text-[14.5px] leading-[1.7]">
      <AnswerText messageId={message.id} text={message.text} sources={message.sources} />
      {message.error && <p className="mt-2 text-[13px] text-bad">{message.error}</p>}
    </div>
  );
}

// Thinking hiện trong lúc AI chưa viết chữ nào: một dòng cố định, không đổi
// theo từng bước để khỏi nhảy chữ.
export function Thinking() {
  return (
    <div
      role="status"
      className="flex items-center gap-2.5 self-start rounded-[12px] border border-rule-soft bg-field px-3.5 py-2.5 text-[13.5px] whitespace-nowrap text-ink-2"
    >
      <SparkIcon className="size-4 flex-none animate-pulse text-blueprint" />
      Đang trả lời
      <span className="inline-flex gap-[3px]" aria-hidden>
        {[0, 200, 400].map((d) => (
          <span
            key={d}
            className="size-[5px] animate-blip rounded-full bg-blueprint"
            style={{ animationDelay: `${d}ms` }}
          />
        ))}
      </span>
    </div>
  );
}

// Câu hỏi gợi ý cho người chưa biết hỏi gì; dùng được với hầu hết tài liệu.
const SUGGESTIONS = [
  "Tóm tắt nội dung chính của tài liệu",
  "Tài liệu có những mốc thời hạn hoặc con số quan trọng nào?",
  "Ai chịu trách nhiệm những việc gì?",
];

// Welcome hiện khi cuộc hỏi đáp chưa có câu nào.
export function Welcome({ onAsk }: { onAsk: (question: string) => void }) {
  return (
    <div className="m-auto flex max-w-[40ch] flex-col items-center px-4 py-8 text-center">
      <span className="mb-3 grid size-11 place-items-center rounded-full bg-blueprint-soft text-blueprint">
        <SparkIcon className="size-5" />
      </span>
      <h3 className="text-[16px] font-semibold">Hỏi bất cứ điều gì về tài liệu này</h3>
      <p className="mt-1 text-[13.5px] leading-relaxed text-ink-3">
        AI trả lời kèm trích dẫn tới đúng trang. Bạn cũng có thể bôi đen một đoạn trong tài liệu để hỏi về đoạn đó.
      </p>
      <div className="mt-5 flex w-full flex-col gap-2">
        {SUGGESTIONS.map((q) => (
          <button
            key={q}
            type="button"
            onClick={() => onAsk(q)}
            className="rounded-[10px] border border-rule bg-white px-3.5 py-2.5 text-left text-[13.5px] text-ink-2 transition-colors hover:border-blueprint/40 hover:text-blueprint"
          >
            {q}
          </button>
        ))}
      </div>
    </div>
  );
}
