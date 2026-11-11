import { useEffect, useEffectEvent, useRef, useState } from "react";

import { api, type Source } from "@/lib/api";

export type StreamStatus = "idle" | "connecting" | "streaming" | "done" | "error";

const FLUSH_MS = 50;

// Kết quả cuối của một lần hỏi, đưa cho onFinish.
export type AnswerResult = {
  text: string;
  sources: Source[];
  status: "done" | "error";
  error: string | null;
};

type Run = {
  docId: string;
  question: string | null;
  text: string;
  sources: Source[];
  status: StreamStatus;
  error: string | null;
};

const newRun = (docId: string, question: string | null): Run => ({
  docId,
  question,
  text: "",
  sources: [],
  // Về idle khi question là null để người gọi không đọc nhầm "done" của lần
  // hỏi trước.
  status: question === null ? "idle" : "connecting",
  error: null,
});

// useAnswerStream gói toàn bộ EventSource của endpoint hỏi đáp. Mỗi khi
// question đổi sang một chuỗi khác null, hook mở một stream mới; null thì
// về idle. Token được gom vào ref và chỉ flush vào state mỗi 50ms, để
// không render lại theo từng token.
//
// Server gửi: sources → token... → done, hoặc error. Sau done/error phải
// đóng stream ngay, nếu không EventSource sẽ tự kết nối lại và hỏi lại.
// onFinish (nếu có) được gọi đúng một lần với toàn bộ câu trả lời.
export function useAnswerStream(docId: string, question: string | null, onFinish?: (result: AnswerResult) => void) {
  const [run, setRun] = useState(() => newRun(docId, question));
  const buffer = useRef("");
  const notifyFinish = useEffectEvent((result: AnswerResult) => onFinish?.(result));

  // Đổi câu hỏi thì làm mới state ngay trong lúc render (thay vì setState
  // trong effect), để không có lần render nào trộn câu hỏi mới với câu trả
  // lời cũ.
  if (run.docId !== docId || run.question !== question) {
    setRun(newRun(docId, question));
  }

  useEffect(() => {
    if (question === null) return;

    buffer.current = "";
    const es = new EventSource(api.askUrl(docId, question));
    let started = false;
    let finished = false;
    let fullText = "";
    let sources: Source[] = [];

    const flush = () => {
      if (!buffer.current) return;
      const chunk = buffer.current;
      buffer.current = "";
      setRun((r) => ({ ...r, text: r.text + chunk }));
    };
    const timer = setInterval(flush, FLUSH_MS);

    const finish = (next: "done" | "error", message?: string) => {
      if (finished) return;
      finished = true;
      es.close();
      clearInterval(timer);
      flush();
      setRun((r) => ({ ...r, status: next, error: message ?? null }));
      notifyFinish({ text: fullText, sources, status: next, error: message ?? null });
    };

    es.addEventListener("sources", (e) => {
      started = true;
      sources = (JSON.parse(e.data) as { chunks: Source[] }).chunks;
      setRun((r) => ({ ...r, sources, status: "streaming" }));
    });
    es.addEventListener("token", (e) => {
      const token = (JSON.parse(e.data) as { text: string }).text;
      buffer.current += token;
      fullText += token;
    });
    es.addEventListener("done", () => finish("done"));
    es.addEventListener("error", (e) => {
      // Event "error" do server gửi là MessageEvent có data; lỗi kết nối của
      // chính EventSource thì không có data.
      if (e instanceof MessageEvent && typeof e.data === "string") {
        const body = JSON.parse(e.data) as { message?: string };
        finish("error", body.message ?? "Máy chủ báo lỗi khi trả lời");
        return;
      }
      // Lỗi trước event đầu tiên là phản hồi JSON có status khác 200 (câu hỏi
      // không hợp lệ, tài liệu chưa sẵn sàng...); EventSource không đọc được
      // nội dung nên chỉ báo chung.
      finish(
        "error",
        started
          ? "Mất kết nối giữa chừng, câu trả lời có thể chưa đầy đủ"
          : "Không gửi được câu hỏi: máy chủ không phản hồi hoặc tài liệu chưa sẵn sàng",
      );
    });

    return () => {
      finished = true;
      es.close();
      clearInterval(timer);
    };
  }, [docId, question]);

  return { text: run.text, sources: run.sources, status: run.status, error: run.error };
}
