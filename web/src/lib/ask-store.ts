import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import type { Source } from "@/lib/api";

// State giao diện dùng chung giữa cột tài liệu và cột hỏi đáp. Dữ liệu
// server (tài liệu, trang, chunk) KHÔNG nằm ở đây mà ở TanStack Query.
// Hội thoại chỉ sống ở trình duyệt (API không lưu), nên được giữ trong
// localStorage để không mất khi tải lại trang.

export type Citation = {
  messageId: string;
  ref: number;
  page: number;
  charStart: number;
  charEnd: number;
  // Tăng mỗi lần bấm, để bấm lại cùng một thẻ vẫn nhấp nháy lại.
  nonce: number;
};

export type Message =
  | { id: string; role: "user"; text: string; quote?: string }
  | { id: string; role: "assistant"; text: string; sources: Source[]; error?: string };

export type Conversation = {
  id: string;
  docId: string;
  title: string;
  createdAt: number;
};

type AskState = {
  // Tài liệu mà currentPage, activeCitation, pendingQuote đang nói tới.
  documentId: string | null;
  currentPage: number;
  activeCitation: Citation | null;
  pendingQuote: string | null;
  messages: Record<string, Message[]>; // theo conversation id
  streaming: boolean;
  conversations: Conversation[];
  activeConversation: string | null;

  enterDocument: (docId: string) => void;
  jumpTo: (docId: string, c: Omit<Citation, "nonce">) => void;
  setCurrentPage: (page: number) => void;
  openCitation: (c: Omit<Citation, "nonce">) => void;
  setPendingQuote: (quote: string | null) => void;
  setStreaming: (streaming: boolean) => void;
  newConversation: (docId: string) => string;
  selectConversation: (id: string) => void;
  addMessage: (conversationId: string, message: Message) => void;
  removeDocument: (docId: string) => void;
};

const TITLE_MAX = 26;

export const newId = () => crypto.randomUUID();

const titleFrom = (text: string) => (text.length > TITLE_MAX ? text.slice(0, TITLE_MAX).trimEnd() + "…" : text);

export const useAskStore = create<AskState>()(
  persist(
    (set) => ({
      documentId: null,
      currentPage: 1,
      activeCitation: null,
      pendingQuote: null,
      messages: {},
      streaming: false,
      conversations: [],
      activeConversation: null,

      // Vào màn hỏi đáp của một tài liệu. Quay lại đúng tài liệu cũ thì giữ
      // trang và trích dẫn; sang tài liệu khác thì về trang 1. Hội thoại đang
      // mở được suy ra khi đọc, xem activeConversationFor.
      enterDocument: (docId) =>
        set((s) =>
          s.documentId === docId
            ? {}
            : { documentId: docId, currentPage: 1, activeCitation: null, pendingQuote: null },
        ),
      // Mở tài liệu ở đúng đoạn, dùng trước khi chuyển sang màn hỏi đáp.
      jumpTo: (docId, c) =>
        set((s) => ({
          documentId: docId,
          pendingQuote: s.documentId === docId ? s.pendingQuote : null,
          activeCitation: { ...c, nonce: (s.activeCitation?.nonce ?? 0) + 1 },
          currentPage: c.page,
        })),
      // Lật trang bằng tay thì bỏ trích dẫn đang chọn.
      setCurrentPage: (page) => set({ currentPage: page, activeCitation: null }),
      openCitation: (c) =>
        set((s) => ({ activeCitation: { ...c, nonce: (s.activeCitation?.nonce ?? 0) + 1 }, currentPage: c.page })),
      setPendingQuote: (quote) => set({ pendingQuote: quote }),
      setStreaming: (streaming) => set({ streaming }),
      newConversation: (docId) => {
        const conv: Conversation = { id: newId(), docId, title: "Hội thoại mới", createdAt: Date.now() };
        set((s) => ({
          conversations: [...s.conversations, conv],
          messages: { ...s.messages, [conv.id]: [] },
          activeConversation: conv.id,
          activeCitation: null,
        }));
        return conv.id;
      },
      selectConversation: (id) => set({ activeConversation: id, activeCitation: null }),
      addMessage: (conversationId, message) =>
        set((s) => {
          const prev = s.messages[conversationId] ?? [];
          // Câu hỏi đầu tiên đặt tên cho hội thoại.
          const conversations =
            prev.length === 0 && message.role === "user"
              ? s.conversations.map((c) => (c.id === conversationId ? { ...c, title: titleFrom(message.text) } : c))
              : s.conversations;
          return { conversations, messages: { ...s.messages, [conversationId]: [...prev, message] } };
        }),
      removeDocument: (docId) =>
        set((s) => {
          const gone = new Set(s.conversations.filter((c) => c.docId === docId).map((c) => c.id));
          const messages = Object.fromEntries(Object.entries(s.messages).filter(([id]) => !gone.has(id)));
          return {
            conversations: s.conversations.filter((c) => !gone.has(c.id)),
            messages,
            activeConversation: s.activeConversation && gone.has(s.activeConversation) ? null : s.activeConversation,
          };
        }),
    }),
    {
      name: "doc-insight:ask",
      version: 1,
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({ conversations: s.conversations, messages: s.messages, activeConversation: s.activeConversation }),
    },
  ),
);

// Hội thoại đang mở của một tài liệu: activeConversation nếu nó thuộc tài
// liệu này, không thì hội thoại mới nhất của tài liệu.
export function activeConversationFor(s: AskState, docId: string): Conversation | undefined {
  const mine = s.conversations.filter((c) => c.docId === docId);
  return mine.find((c) => c.id === s.activeConversation) ?? mine.at(-1);
}
