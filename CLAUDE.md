# Doc Insight

Hệ thống hỏi đáp tài liệu nội bộ. Upload PDF, hỏi bằng tiếng
Việt, câu trả lời kèm trích dẫn chỉ tới số trang trong tài
liệu gốc.

## Kiến trúc

```
web/  React 19 + Vite, React Router, TypeScript (SPA, build ra file tĩnh)
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

- Đặc tả giao diện: `docs/design/mockup.html` (mở bằng trình duyệt).
  Bố cục, màu, chữ và luồng tương tác của `web/` bám theo file này. Khác
  mockup: cột trái vẽ trang PDF thật bằng pdf.js (`react-pdf`) từ
  `GET /api/documents/{id}/file`, không phải văn bản trích đặt trên tờ giấy giả.

- Dữ liệu từ server → TanStack Query. Query key dạng mảng:
  `['documents']`, `['document', id]`, `['pages', id, pageNo]`,
  `['chunks', id]`. Khai báo ở `web/src/lib/query-client.ts`.
- State giao diện dùng chung giữa hai cột → Zustand store
  `ask-store.ts`.
- State chỉ một component quan tâm → `useState` tại chỗ.
- Không bao giờ nhét dữ liệu server vào Zustand.
- Route khai báo một chỗ ở `web/src/main.tsx` (`createBrowserRouter`).
  `RouterProvider` import từ `react-router/dom`, còn lại từ `react-router`.
- Không setState đồng bộ trong effect (luật `react-hooks/set-state-in-effect`):
  state phụ thuộc prop thì tính ra khi render hoặc reset ngay trong lúc render,
  setState chỉ nằm trong callback (sự kiện, EventSource, timer).
- Mọi `EventSource` phải có `es.close()` trong hàm cleanup, và đóng
  ngay khi nhận `done`/`error` — nếu không trình duyệt tự kết nối lại
  và hỏi lại từ đầu.
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
- **Tô sáng trên PDF thật bằng so khớp văn bản**: offset của chunk tính
  trên văn bản do Go trích, còn text layer của pdf.js là văn bản khác cách
  ngắt dòng. `web/src/lib/pdf-highlight.ts` chuẩn hoá cả hai (NFKC, chữ
  thường, chỉ giữ chữ và số), tìm đoạn nguồn rồi ánh xạ về từng item.
  Callback truyền cho `<Page>` của react-pdf phải cố định (`useCallback`),
  nếu không text layer vẽ lại liên tục.
- **Bảng `pages` lưu văn bản từng trang**, cùng transaction với
  chunk. Offset của chunk tính theo rune trên text này; frontend
  cắt bằng `Array.from`, không dùng `slice` trên UTF-16. Tài liệu
  xử lý trước migration 000003 không có trang, phải tải lên lại.
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
cd web && npm run dev                      # frontend (Vite) ở http://localhost:3000
cd web && npm run build && npm run lint    # kiểm tra frontend
```

Test tích hợp cần Postgres có pgvector (db của docker compose):

```bash
cd api && TEST_DATABASE_URL='postgres://docinsight:docinsight@localhost:5432/docinsight?sslmode=disable' go test -race ./...
```

Mỗi test tự tạo rồi xoá một database tạm. Đặt thêm `TEST_POOLER_URL`
(chuỗi đi qua PgBouncer transaction mode) để chạy giống chuỗi
`-pooler` của Neon.

Chưa có: Makefile (`make migrate`, `make seed`), Dockerfile cho `web/`.

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
| `LLM_EMBED_MODEL` | Model embedding của Gemini, mặc định `gemini-embedding-001` (phải ra được 768 chiều) |
| `LLM_CHAT_MODEL` | Model trả lời của Gemini, danh sách phân tách bằng dấu phẩy, thử lần lượt khi model trước hết quota (429), không còn (404) hay lỗi 5xx. Mặc định `gemini-3.5-flash,gemini-3.5-flash-lite,gemini-3.1-flash-lite` |
| `ALLOWED_ORIGINS` | CORS, phân tách bằng dấu phẩy, mặc định `http://localhost:3000`; `*` là mọi origin |
| `VITE_API_URL` | (web) Địa chỉ API, mặc định `http://localhost:8080`. Nhúng vào bundle lúc build; xem `web/.env.example` |

Xem `.env.example` để có mẫu đầy đủ.

## Lưu ý khi sửa code

- Header SSE phải có `X-Accel-Buffering: no`, nếu thiếu thì
  proxy đệm và streaming không chạy.
- Đổi trang xong đừng scroll ngay — phần tử chưa có trong
  DOM. Effect phải phụ thuộc cả `[activeCitation, pageData]`.
- Lưu chunk phải nằm trong một transaction.
- Câu trả lời được dùng kiến thức chung, nhưng phần đó phải nằm sau nhãn
  `OutsideLabel` ("Ngoài tài liệu:", trong `service/ask.go`) và không có
  `[[n]]`. Frontend (`answer-text.tsx`) dựa vào đúng chuỗi này để hiện khung
  riêng; đổi nhãn thì đổi cả hai nơi.
