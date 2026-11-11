// Lớp gọi API của doc-insight. Mọi URL đều dựng từ VITE_API_URL.

export const API_URL = (import.meta.env.VITE_API_URL ?? "http://localhost:8080").replace(/\/+$/, "");

export type DocumentStatus = "pending" | "extracting" | "chunking" | "embedding" | "ready" | "failed";

export type Document = {
  id: string;
  filename: string;
  size_bytes: number;
  page_count: number | null;
  chunk_count: number;
  status: DocumentStatus;
  progress: number;
  error: string | null;
  // Mô hình embedding đã dùng; null = không rõ (xử lý trước khi API ghi lại).
  embed_model: string | null;
  // Đã xử lý bằng mô hình khác mô hình đang chạy: tìm kiếm sẽ sai, phải xử lý lại.
  needs_reprocess: boolean;
  processing_started_at: string | null;
  created_at: string;
  updated_at: string;
  processed_at: string | null;
};

export type Chunk = {
  id: number;
  index: number;
  page_number: number;
  char_start: number;
  char_end: number;
  token_count: number | null;
  content: string;
};

export type PageData = {
  document_id: string;
  page_count: number;
  page_number: number;
  text: string;
  chunks: Chunk[];
};

// Một đoạn nguồn trong event `sources`; câu trả lời trích dẫn nó bằng [[ref]].
export type Source = {
  ref: number;
  // Thứ tự đoạn trong tài liệu (từ 0); tin nhắn cũ lưu trước khi có trường này thì thiếu.
  chunk?: number;
  page: number;
  char_start: number;
  char_end: number;
  preview: string;
  score: number;
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly requestId?: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(API_URL + path, init);
  } catch {
    throw new ApiError("Không kết nối được tới máy chủ", 0);
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string; request_id?: string } | null;
    throw new ApiError(body?.error ?? `Lỗi ${res.status}`, res.status, body?.request_id);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  listDocuments: () => request<{ documents: Document[] }>("/api/documents").then((r) => r.documents),
  getDocument: (id: string) => request<Document>(`/api/documents/${id}`),
  deleteDocument: (id: string) => request<void>(`/api/documents/${id}`, { method: "DELETE" }),
  reprocessDocument: (id: string) => request<Document>(`/api/documents/${id}/reprocess`, { method: "POST" }),
  getPage: (id: string, pageNo: number) => request<PageData>(`/api/documents/${id}/pages/${pageNo}`),
  listChunks: (id: string) => request<{ chunks: Chunk[] }>(`/api/documents/${id}/chunks`).then((r) => r.chunks),
  fileUrl: (id: string) => `${API_URL}/api/documents/${id}/file`,
  askUrl: (id: string, question: string) => `${API_URL}/api/documents/${id}/ask?q=${encodeURIComponent(question)}`,
};

// uploadDocument dùng XHR thay vì fetch để báo được tiến độ gửi file.
export function uploadDocument(
  file: File,
  onProgress: (percent: number) => void,
  signal?: AbortSignal,
): Promise<{ id: string; status: DocumentStatus }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_URL}/api/documents`);
    xhr.responseType = "json";
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100));
    };
    xhr.onload = () => {
      const body = xhr.response as { id?: string; status?: DocumentStatus; error?: string; request_id?: string } | null;
      if (xhr.status >= 200 && xhr.status < 300 && body?.id) {
        resolve({ id: body.id, status: body.status ?? "pending" });
      } else {
        reject(new ApiError(body?.error ?? `Lỗi ${xhr.status}`, xhr.status, body?.request_id));
      }
    };
    xhr.onerror = () => reject(new ApiError("Không kết nối được tới máy chủ", 0));
    xhr.onabort = () => reject(new ApiError("Đã huỷ", 0));
    signal?.addEventListener("abort", () => xhr.abort());
    const form = new FormData();
    form.append("file", file);
    xhr.send(form);
  });
}

export const isProcessing = (s: DocumentStatus) => s !== "ready" && s !== "failed";
