# Doc Insight

Hệ thống hỏi đáp tài liệu nội bộ. Upload PDF, hỏi bằng tiếng
Việt, câu trả lời kèm trích dẫn chỉ tới số trang trong tài
liệu gốc.

## Kiến trúc

```
web/  Next.js 15 App Router, TypeScript
api/  Go 1.22, chi, pgx/v5
db    PostgreSQL 16 + pgvector
```

Luồng chính: upload PDF → trích text giữ số trang → cắt chunk
→ embedding → pgvector. Hỏi → embed câu hỏi → top-5 chunk →
prompt → stream câu trả lời qua SSE kèm trích dẫn.

## Quy ước Go

- Phân tầng: handler → service → repository. Không gọi ngược.
- SQL chỉ nằm trong `internal/repository`. Service không viết
  SQL.
- Interface khai báo ở nơi dùng (service), không ở nơi cài
  đặt (llm).
- Mọi hàm có I/O nhận `ctx context.Context` làm tham số đầu.
- Bọc lỗi bằng `fmt.Errorf("...: %w", err)`. Không nuốt lỗi,
  không `panic` trong code thường.
- Lỗi nghiệp vụ khai báo thành biến: `ErrDocNotFound`,
  `ErrInvalidFile`. Handler map sang HTTP status ở một chỗ.
- Dùng `log/slog`. Không `fmt.Println`.
- Không bắn goroutine trần cho việc nền — dùng worker pool có
  giới hạn.
- Test dùng table-driven. Test service phải chạy được offline
  qua `MockProvider`.

## Quy ước frontend

- Dữ liệu từ server → TanStack Query. Query key dạng mảng:
  `['documents']`, `['document', id]`, `['pages', id, pageNo]`.
- State giao diện dùng chung giữa hai cột → Zustand store
  `ask-store.ts`.
- State chỉ một component quan tâm → `useState` tại chỗ.
- Không bao giờ nhét dữ liệu server vào Zustand.
- Server Component là mặc định. Chỉ thêm `"use client"` khi
  thật sự cần hook hoặc sự kiện.
- Mọi `EventSource` phải có `es.close()` trong hàm cleanup.
- Token stream gom vào `useRef`, flush vào state mỗi 50ms.
  Không setState mỗi token.
- UI dùng shadcn/ui và Tailwind. Không viết CSS rời.

## Quyết định đã chốt — đừng đổi nếu không hỏi

- **SSE thay vì WebSocket**: luồng một chiều, trình duyệt tự
  kết nối lại, qua proxy dễ hơn.
- **pgvector thay vì vector DB riêng**: bớt một thành phần
  vận hành, dùng chung transaction với dữ liệu thường.
- **Chunk giữ `page_number`, `char_start`, `char_end`**: đây
  là điều kiện để nhảy tới vị trí trong tài liệu. Không được
  bỏ.
- **Interface `LLMProvider` với hai implementation**: mock để
  test offline và demo không cần mạng.
- **Chunk 500 token, chồng lấn 50**: nhỏ quá mất ngữ cảnh, to
  quá vector bị pha loãng.
- **Event `sources` gửi trước token đầu tiên**: frontend cần
  dữ liệu nguồn sẵn để parse `[[n]]`.

## Lệnh hay dùng

```bash
docker compose up --build                  # chạy db + api; migration tự chạy khi api khởi động
cd api && go test ./... && go vet ./...    # kiểm tra api (test tích hợp bị skip)
```

Test tích hợp cần Postgres có pgvector (db của docker compose):

```bash
cd api && TEST_DATABASE_URL='postgres://docinsight:docinsight@localhost:5432/docinsight?sslmode=disable' go test -race ./...
```

Mỗi test tự tạo rồi xoá một database tạm. Đặt thêm `TEST_POOLER_URL`
(chuỗi đi qua PgBouncer transaction mode) để chạy giống chuỗi
`-pooler` của Neon.

Chưa có: Makefile (`make migrate`, `make seed`), thư mục `web/`.

## Biến môi trường

| Tên | Mô tả |
|---|---|
| `DATABASE_URL` | Postgres connection string (bắt buộc) |
| `PORT` | Cổng server, mặc định 8080. Đọc từ env, không hardcode |
| `DB_MAX_CONNS` | Số kết nối tối đa của pgxpool, mặc định 10 |
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error`; mặc định `info` |
| `STORAGE_DIR` | Nơi lưu file PDF, mặc định `storage` (Docker: `/data/uploads`) |
| `LLM_PROVIDER` | `mock` hoặc `gemini`, mặc định `mock` |
| `LLM_API_KEY` | Gemini API key, bắt buộc khi `LLM_PROVIDER=gemini` |
| `LLM_EMBED_MODEL` | Model embedding của Gemini, mặc định `text-embedding-004` (phải ra được 768 chiều) |
| `LLM_CHAT_MODEL` | Model trả lời của Gemini, mặc định `gemini-2.0-flash` |
| `ALLOWED_ORIGINS` | CORS, phân tách bằng dấu phẩy, mặc định `http://localhost:3000`; `*` là mọi origin |

Xem `.env.example` để có mẫu đầy đủ.

## Lưu ý khi sửa code

- Header SSE phải có `X-Accel-Buffering: no`, nếu thiếu thì
  proxy đệm và streaming không chạy.
- Đổi trang xong đừng scroll ngay — phần tử chưa có trong
  DOM. Effect phải phụ thuộc cả `[activeCitation, pageData]`.
- Lưu chunk phải nằm trong một transaction.
