import { useEffect, useRef, useState } from "react";
import { useShallow } from "zustand/react/shallow";

import { AnswerText } from "@/components/ask/answer-text";
import { Composer } from "@/components/ask/composer";
import { AssistantMessage, Thinking, UserBubble, Welcome } from "@/components/ask/messages";
import { PlusIcon } from "@/components/icons";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useAnswerStream } from "@/hooks/use-answer-stream";
import type { Document } from "@/lib/api";
import { activeConversationFor, newId, useAskStore, type Message } from "@/lib/ask-store";
import { buildQuestion } from "@/lib/question";

const NO_MESSAGES: Message[] = [];

// Câu hỏi đang chờ trả lời: thuộc hội thoại nào và id của tin trả lời.
type Pending = { conversationId: string; messageId: string; question: string };

// Cột phải: chọn cuộc hỏi đáp, khung chat và ô nhập.
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
  // Stream xong (hoặc lỗi) thì chuyển câu trả lời vào lịch sử hội thoại.
  const stream = useAnswerStream(doc.id, pending?.question ?? null, (result) => {
    if (!pending) return;
    addMessage(pending.conversationId, {
      id: pending.messageId,
      role: "assistant",
      text: result.text,
      sources: result.sources,
      error: result.error ?? undefined,
    });
    setPending(null);
    setStreaming(false);
  });
  const threadRef = useRef<HTMLDivElement>(null);

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
    const question = buildQuestion(text, quote);
    setPending({ conversationId, messageId: newId(), question });
  };

  const NEW = "__new__";
  return (
    <section className="flex min-w-0 flex-[1_1_44%] flex-col bg-paper">
      <div className="flex h-[52px] flex-none items-center gap-2 border-b border-rule-soft px-4">
        <Select
          value={active?.id ?? NEW}
          onValueChange={(v) => (v === NEW ? newConversation(doc.id) : selectConversation(v))}
        >
          <SelectTrigger
            aria-label="Chọn cuộc hỏi đáp"
            className="h-8 max-w-[70%] min-w-0 border-transparent bg-transparent px-2 text-ui font-medium shadow-none hover:bg-field"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {!active && <SelectItem value={NEW}>Cuộc hỏi đáp mới</SelectItem>}
            {conversations.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.title}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => newConversation(doc.id)}
          disabled={!!pending}
          className="ml-auto h-8 gap-1.5 px-2.5 text-small text-ink-2 hover:bg-field"
        >
          <PlusIcon className="size-3.5 stroke-2" />
          Cuộc mới
        </Button>
      </div>

      <div ref={threadRef} className="flex flex-1 flex-col gap-5 overflow-y-auto px-5 py-6">
        {messages.length === 0 && !live ? (
          <Welcome onAsk={(q) => send(q, null)} />
        ) : (
          <>
            {messages.map((m) =>
              m.role === "user" ? <UserBubble key={m.id} message={m} /> : <AssistantMessage key={m.id} message={m} />,
            )}
            {live &&
              (stream.text === "" && (stream.status === "connecting" || stream.status === "streaming") ? (
                <Thinking />
              ) : (
                <div className="max-w-[93%] self-start text-body leading-[1.7]">
                  <AnswerText messageId={live.messageId} text={stream.text} sources={stream.sources} streaming />
                </div>
              ))}
          </>
        )}
      </div>

      <Composer disabled={!!pending} onSend={send} />
    </section>
  );
}
