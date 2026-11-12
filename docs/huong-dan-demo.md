# Hướng dẫn demo Doc Insight

Tài liệu này giúp bạn: bật toàn bộ app bằng một lệnh, nạp dữ liệu mẫu, chạy
kịch bản demo, chạy test, và giải thích được đường đi của SSE trong code.

## 1. Bật toàn bộ app

Cần: Docker Desktop. Muốn dùng AI thật thì cần thêm Gemini API key
(https://aistudio.google.com/apikey); không có key thì app chạy bằng mock.

```bash
cp .env.example .env          # rồi sửa LLM_PROVIDER=gemini, LLM_API_KEY=<key>
docker compose up -d --build  # bật db + api + web
```

| Service | Địa chỉ | Ghi chú |
|---|---|---|
| web | http://localhost:3000 | React build tĩnh, phục vụ qua nginx |
| api | http://localhost:8080 | Go; migration tự chạy khi khởi động |
| db  | localhost:5432 | Postgres 16 + pgvector (user/pass/db: `docinsight`) |

Kiểm tra: `docker compose ps` (cả ba `running`, api `healthy`),
`curl localhost:8080/healthz` → `{"status":"ok","db":"ok"}`.

Nếu đang chạy `npm run dev` ở cổng 3000 thì tắt đi trước, hoặc đặt
`WEB_PORT=3001` trong `.env` (khi đó thêm `http://localhost:3001` vào
`ALLOWED_ORIGINS`).

### Nạp dữ liệu mẫu

```bash
cd api && go run ./cmd/seed
```

Lệnh tải các PDF trong `api/eval/docs` lên API như người dùng thật và đợi xử lý
xong (vài giây). Chạy lại không tạo bản trùng; tài liệu xử lý bằng mô hình cũ
thì được xử lý lại.

### Làm lại từ đầu

```bash
docker compose down -v        # XOÁ toàn bộ dữ liệu (DB + file PDF)
docker compose up -d --build && (cd api && go run ./cmd/seed)
```

### Xem bên trong DB

```bash
docker compose exec db psql -U docinsight -d docinsight
\dt                                            -- documents, chunks, pages, schema_migrations
SELECT filename, status, page_count, chunk_count, embed_model FROM documents;
SELECT chunk_index, page_number, char_start, char_end, left(content, 60) FROM chunks ORDER BY chunk_index;
```

Migration nằm ở `api/migrations`, chạy tuần tự theo số và ghi vào
`schema_migrations`: `000001_documents` → `000002_chunks` (vector 768 chiều,
index ivfflat cosine) → `000003_pages` (văn bản từng trang để tô sáng).

## 2. Kịch bản demo (3–5 phút)

1. **Danh sách** (http://localhost:3000): 2 tài liệu mẫu ở thanh trái.
2. **Tải lên**: bấm "Tải tài liệu lên", kéo một PDF vào → xem tiến độ
   (đọc chữ → chia đoạn → lập chỉ mục) → mở được khi xong.
3. **Hỏi đáp**: mở `phu-luc-dai`, hỏi "Điều 208 quy định gì?" → chữ hiện dần
   (SSE) → chip `1 tr.3`.
4. **Trích dẫn**: bấm chip → cột trái nhảy tới trang 3, tô sáng đúng Điều 208.
5. **Bôi chọn để hỏi**: bôi đen một đoạn trên PDF → "Hỏi về đoạn này" → gửi.
6. **Ngoài tài liệu**: hỏi "Thủ đô của Pháp là gì?" → nói rõ tài liệu không
   đề cập, phần kiến thức chung nằm trong khung "Ngoài tài liệu".
7. **Nội dung**: tab Nội dung → AI đã chia tài liệu thành đoạn thế nào; tìm
   "dieu 213" (không dấu vẫn ra).
8. **Chất lượng**: `cd api && go run ./cmd/eval` → 12/12, nguồn đúng 100%.

Mẹo: gói Gemini miễn phí có giới hạn lượt/ngày. Trước buổi demo nên bật
billing cho key, hoặc chạy `LLM_PROVIDER=mock` (câu trả lời là mẫu, nhưng mọi
tương tác vẫn chạy).

## 3. Chạy test

```bash
# Backend: test đơn vị (không cần DB; test tích hợp tự bỏ qua)
cd api && go test ./... && go vet ./...

# Backend: kèm test tích hợp với Postgres thật + phát hiện data race
cd api && TEST_DATABASE_URL='postgres://docinsight:docinsight@localhost:5432/docinsight?sslmode=disable' go test -race ./...

# Một package / một test, in chi tiết
cd api && go test -v -run TestScore ./internal/eval

# Frontend
cd web && npm test                 # Vitest
cd web && npm run lint && npm run build && npm run format:check

# Chất lượng câu trả lời trên API đang chạy (tốn 1 lượt model mỗi câu)
cd api && go run ./cmd/eval -min 0.9
```

Có gì để xem trong test:

- **Table-driven**: `api/internal/service/ask_test.go` (`TestAskRejects`) — mỗi
  dòng một trường hợp, một vòng lặp chạy hết.
- **Fake thay dịch vụ thật**: `fakeRepo`, `fakeQueue`, `llm.MockProvider` → test
  service chạy offline.
- **Tích hợp**: `api/internal/repository/*_test.go` — mỗi test tạo một database
  tạm, chạy migration thật, rồi xoá.
- **HTTP**: `api/internal/handler/ask_test.go` — gọi router bằng `httptest`,
  đọc và kiểm tra từng event SSE.
- **Frontend**: `web/src/lib/*.test.ts` — tô sáng PDF, tách câu trả lời, trạng
  thái tài liệu, giới hạn câu hỏi.

## 4. SSE trong code: đi từ server tới trình duyệt

SSE (Server-Sent Events) là một response HTTP **không đóng**, server ghi dần
từng "event" dạng chữ:

```
event: sources
data: {"chunks":[{"ref":1,"chunk":5,"page":3,...}]}

event: token
data: {"text":"Điều 208 quy định"}

event: done
data: {"tokens":4,"ms":3519}
```

Thứ tự luôn là `sources` → `token`… → `done` (hoặc `error`). `sources` đi trước
để trình duyệt có sẵn nguồn khi gặp `[[1]]` trong chữ.

### Bước 1 — Handler ghi event (`api/internal/handler/ask.go`)

```go
func (s *sseWriter) event(name string, v any) error {
	select {
	case <-s.ctx.Done(): // client đã đóng tab: dừng ghi
		return s.ctx.Err()
	default:
	}
	data, err := json.Marshal(v)
	...
	if !s.started { // header chỉ ghi ở event đầu tiên
		h := s.w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Accel-Buffering", "no") // thiếu thì nginx đệm, stream không chạy
		s.w.WriteHeader(http.StatusOK)
		s.started = true
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
	s.flusher.Flush() // đẩy ngay xuống client, không chờ đầy buffer
	return nil
}
```

Điểm cần nói được:

- `Flush()` là thứ làm nó "stream": thiếu thì Go gom hết rồi mới gửi.
- Header ghi **trễ** (ở event đầu): lỗi xảy ra trước đó vẫn trả được JSON với
  đúng HTTP status (400, 404, 409).
- `X-Accel-Buffering: no` tắt bộ đệm của nginx/proxy.
- Ở `Ask` (cùng file): xong thì gửi `done`; lỗi giữa chừng thì gửi event
  `error`; lỗi phía máy chủ trước event đầu cũng gửi thành event `error` vì
  `EventSource` không đọc được body của response khác 200.

### Bước 2 — Service chuyển token từ LLM sang writer (`api/internal/service/ask.go`)

```go
func (s *AskService) stream(ctx context.Context, prompt string, w AnswerWriter) (int, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make(chan string, 32)
	errc := make(chan error, 1)
	go func() { // LLM ghi token vào channel
		errc <- s.llm.Stream(streamCtx, prompt, out)
		close(out)
	}()

	for tok := range out { // đọc token và ghi ra SSE
		if err := w.WriteToken(tok); err != nil {
			cancel() // client đã đi: huỷ lời gọi LLM, khỏi tốn tiền
			...
		}
	}
	streamErr := <-errc
	...
}
```

Mô hình **producer/consumer qua channel**: goroutine gọi LLM là producer,
vòng `for range` là consumer. Goroutine này sống trong phạm vi request (luôn
kết thúc khi `Stream` trả về), nên không trái quy tắc "không bắn goroutine
trần cho việc nền". `cancel()` lan xuống request HTTP tới Gemini.

### Bước 3 — Gemini cũng trả SSE (`api/internal/llm/gemini_chat.go`)

Gọi `streamGenerateContent?alt=sse`, đọc từng dòng `data: {...}` bằng
`bufio.Scanner`, lấy phần chữ (bỏ phần "thought" của model) và gửi vào channel.
Chỉ retry / đổi model **trước** token đầu tiên — sau đó người dùng đã thấy chữ,
gọi lại sẽ ra câu trả lời trùng lặp.

### Bước 4 — Trình duyệt nhận (`web/src/hooks/use-answer-stream.ts`)

```ts
const es = new EventSource(api.askUrl(docId, question));
const timer = setInterval(flush, 50);           // flush 50ms một lần

es.addEventListener("sources", (e) => { /* lưu nguồn, status = streaming */ });
es.addEventListener("token", (e) => {
  buffer.current += JSON.parse(e.data).text;      // gom vào ref, KHÔNG setState
});
es.addEventListener("done", () => finish("done"));
es.addEventListener("error", (e) => { /* lỗi của server hoặc mất kết nối */ });

return () => { es.close(); clearInterval(timer); }; // cleanup khi đổi câu/rời trang
```

Điểm cần nói được:

- **Gom token vào `useRef`, flush 50ms**: model có thể gửi hàng trăm token/giây;
  setState mỗi token làm React render lại liên tục.
- **`es.close()` ngay khi `done`/`error`**: `EventSource` tự kết nối lại khi
  stream đóng — không đóng thì nó hỏi lại câu đó từ đầu.
- **Event `error` có hai loại**: do server gửi (có `data`) và lỗi kết nối của
  chính `EventSource` (không có `data`) — phân biệt bằng
  `e instanceof MessageEvent`.

### Vì sao SSE mà không WebSocket?

Câu trả lời chỉ đi một chiều (server → trình duyệt); SSE là HTTP thường nên đi
qua proxy/CDN dễ, trình duyệt tự kết nối lại, không cần thư viện. WebSocket
hợp khi cần hai chiều liên tục (chat nhiều người, cộng tác thời gian thực).
Hạn chế của `EventSource`: chỉ gửi được GET (câu hỏi nằm trong query string),
không đặt được header tuỳ ý — muốn gửi kèm lịch sử hội thoại thì chuyển sang
`fetch` + đọc stream.

Thử tận tay bằng curl (`-N` = không đệm):

```bash
ID=$(curl -s localhost:8080/api/documents | jq -r '.documents[0].id')
curl -N "localhost:8080/api/documents/$ID/ask?q=$(jq -rn --arg q 'Tóm tắt tài liệu' '$q|@uri')"
```
