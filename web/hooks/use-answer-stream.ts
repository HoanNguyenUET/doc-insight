"use client";

import { useEffect, useRef, useState } from "react";

import { api, type Source } from "@/lib/api";

export type StreamStatus = "idle" | "connecting" | "streaming" | "done" | "error";

const FLUSH_MS = 50;

// useAnswerStream gói toàn bộ EventSource của endpoint hỏi đáp. Mỗi khi
// question đổi sang một chuỗi khác null, hook mở một stream mới; null thì
// không làm gì. Token được gom vào ref và chỉ flush vào state mỗi 50ms, để
// không render lại theo từng token.
//
// Server gửi: sources → token... → done, hoặc error. Sau done/error phải
// đóng stream ngay, nếu không EventSource sẽ tự kết nối lại và hỏi lại.
export function useAnswerStream(docId: string, question: string | null) {
  const [text, setText] = useState("");
  const [sources, setSources] = useState<Source[]>([]);
  const [status, setStatus] = useState<StreamStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const buffer = useRef("");

  useEffect(() => {
    if (question === null) {
      // Về idle để người gọi không đọc nhầm "done" của lần hỏi trước.
      setStatus("idle");
      return;
    }

    buffer.current = "";
    setText("");
    setSources([]);
    setError(null);
    setStatus("connecting");

    const es = new EventSource(api.askUrl(docId, question));
    let started = false;
    let finished = false;

    const flush = () => {
      if (!buffer.current) return;
      const chunk = buffer.current;
      buffer.current = "";
      setText((t) => t + chunk);
    };
    const timer = setInterval(flush, FLUSH_MS);

    const finish = (next: "done" | "error", message?: string) => {
      if (finished) return;
      finished = true;
      es.close();
      clearInterval(timer);
      flush();
      if (message) setError(message);
      setStatus(next);
    };

    es.addEventListener("sources", (e) => {
      started = true;
      setSources((JSON.parse(e.data) as { chunks: Source[] }).chunks);
      setStatus("streaming");
    });
    es.addEventListener("token", (e) => {
      buffer.current += (JSON.parse(e.data) as { text: string }).text;
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

  return { text, sources, status, error };
}
