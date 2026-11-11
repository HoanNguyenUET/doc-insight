"use client";

import { useEffect, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";

import { AnswerText } from "@/components/ask/answer-text";
import { Composer } from "@/components/ask/composer";
import { Button } from "@/components/ui/button";
import { useAnswerStream } from "@/hooks/use-answer-stream";
import type { Document } from "@/lib/api";
import { activeConversationFor, newId, useAskStore, type Message } from "@/lib/ask-store";
import { displayName } from "@/lib/format";
import { cn } from "@/lib/utils";

const NO_MESSAGES: Message[] = [];

// Câu hỏi đang chờ trả lời: thuộc hội thoại nào và id của tin trả lời.
type Pending = { conversationId: string; messageId: string; question: string };

// Cột phải: tab hội thoại, khung chat và ô nhập.
export function ChatPanel({ doc }: { doc: Document }) {
  const conversations = useAskStore(useShallow((s) => s.conversations.filter((c) => c.docId === doc.id)));
  const active = useAskStore((s) => activeConversationFor(s, doc.id));
  const messages = useAskStore((s) => (active ? s.messages[active.id] : undefined) ?? NO_MESSAGES);
  const { newConversation, selectConversation, addMessage, setStreaming, setPendingQuote } = useAskStore(
    useShallow((s) => ({
      newConversation: s.newConversation,
      selectConversation: s.selectConversation,
      addMessage: s.addMessage,
      setStreaming: s.setStreaming,
      setPendingQuote: s.setPendingQuote,
    })),
  );

  const [pending, setPending] = useState<Pending | null>(null);
  const stream = useAnswerStream(doc.id, pending?.question ?? null);
  const threadRef = useRef<HTMLDivElement>(null);

  // Stream xong (hoặc lỗi) thì chuyển câu trả lời vào lịch sử hội thoại.
  useEffect(() => {
    if (!pending || (stream.status !== "done" && stream.status !== "error")) return;
    addMessage(pending.conversationId, {
      id: pending.messageId,
      role: "assistant",
      text: stream.text,
      sources: stream.sources,
      error: stream.error ?? undefined,
    });
    setPending(null);
    setStreaming(false);
  }, [pending, stream.status, stream.text, stream.sources, stream.error, addMessage, setStreaming]);

  // Rời màn hình giữa chừng thì hook tự đóng stream; chỉ cần tắt cờ.
  useEffect(() => () => setStreaming(false), [setStreaming]);

  const live = pending && active?.id === pending.conversationId ? pending : null;

  useEffect(() => {
    const el = threadRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages.length, stream.text, live]);

  const send = (text: string, quote: string | null) => {
    if (pending) return;
    const conversationId = active?.id ?? newConversation(doc.id);
    addMessage(conversationId, { id: newId(), role: "user", text, quote: quote ?? undefined });
    setPendingQuote(null);
    setStreaming(true);
    // Đoạn trích nằm trong câu hỏi gửi lên để retrieval tìm đúng vùng đó.
    const question = quote ? `Về đoạn trích «${quote}»: ${text}` : text;
    setPending({ conversationId, messageId: newId(), question });
  };

  return (
    <section className="flex min-w-0 flex-[1_1_44%] flex-col bg-paper">
      <div className="flex h-[50px] flex-none items-center justify-between gap-3 border-b border-rule-soft px-[18px]">
        <h2 className="truncate text-[14.5px] font-semibold">{displayName(doc.filename)}</h2>
        {conversations.length > 0 && (
          <span className="text-[12.5px] whitespace-nowrap text-ink-3">{conversations.length} hội thoại</span>
        )}
      </div>

      <div
        role="tablist"
        className="flex flex-none items-stretch gap-0.5 overflow-x-auto border-b border-rule px-3 [scrollbar-width:none]"
      >
        {conversations.map((c) => (
          <button
            key={c.id}
            role="tab"
            aria-selected={c.id === active?.id}
            onClick={() => selectConversation(c.id)}
            className={cn(
              "-mb-px rounded-t-md border-b-2 border-transparent px-[13px] pt-[9px] pb-2 text-[13.5px] whitespace-nowrap text-ink-2 hover:bg-field hover:text-ink",
              c.id === active?.id && "border-blueprint font-medium text-blueprint",
            )}
          >
            {c.title}
          </button>
        ))}
        <Button
          variant="ghost"
          size="icon"
          onClick={() => newConversation(doc.id)}
          title="Hội thoại mới"
          aria-label="Hội thoại mới"
          className="size-8 self-center text-base text-ink-3 hover:bg-field"
        >
          +
        </Button>
      </div>

      <div ref={threadRef} className="flex flex-1 flex-col gap-[18px] overflow-y-auto px-5 py-[22px]">
        {messages.length === 0 && !live ? (
          <div className="m-auto max-w-[34ch] px-5 py-10 text-center text-sm leading-relaxed text-ink-3">
            Chưa có câu hỏi nào cho tài liệu này. Gõ câu hỏi bên dưới, hoặc bôi đen một đoạn trong tài liệu để hỏi về
            đoạn đó.
          </div>
        ) : (
          <>
            {messages.map((m) => (m.role === "user" ? <UserBubble key={m.id} message={m} /> : <AssistantMessage key={m.id} message={m} />))}
            {live && (
              <div className="max-w-[93%] self-start text-[14.5px] leading-[1.68]">
                <AnswerText messageId={live.messageId} text={stream.text} sources={stream.sources} streaming />
              </div>
            )}
          </>
        )}
      </div>

      <Composer disabled={!!pending} onSend={send} />
    </section>
  );
}

function UserBubble({ message }: { message: Extract<Message, { role: "user" }> }) {
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

function AssistantMessage({ message }: { message: Extract<Message, { role: "assistant" }> }) {
  return (
    <div className="max-w-[93%] self-start text-[14.5px] leading-[1.68]">
      <AnswerText messageId={message.id} text={message.text} sources={message.sources} />
      {message.error && <p className="mt-2 text-[13px] text-bad">{message.error}</p>}
    </div>
  );
}
