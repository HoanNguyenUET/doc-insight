# Devlog 01 — Nền tảng API: compose, migration, healthz

Phạm vi: docker compose (Postgres 16 + pgvector, API Go), migration bảng
`documents`/`chunks`, config từ env, pgxpool, `GET /healthz`, Dockerfile
nhiều tầng. Sau đó viết test và kiểm chứng; phần đáng kể nhất là **hai
vấn đề chỉ lộ ra khi test**, kể ở mục 2 và 3.

---

## 1. Những quyết định thiết kế

| Quyết định | Lý do |
|---|---|
| Server đọc cổng từ `PORT`, mặc định 8080 | Render cấp cổng động. Kiểm chứng: unit test + chạy container với `PORT=9123`. |
| `/healthz` ping DB với timeout 3 s, trả **503** khi lỗi | Load balancer dựa vào status code, không đọc body. Timeout để một DB treo (Neon đang đánh thức compute) không kéo healthz treo theo; có test riêng cho trường hợp này. |
| Ping lúc khởi động cho tới 15 s, healthz chỉ 3 s | Neon scale-to-zero: lần kết nối đầu có thể chậm. Khởi động chịu chờ, healthz thì phải nhanh. |
| Migration nhúng vào binary (`embed`), chạy khi server khởi động | Render + Neon không cần bước migrate riêng; tên file theo chuẩn golang-migrate để đổi công cụ sau này dễ. |
| `chunks.embedding` cho phép NULL | Pipeline có bước cắt đoạn trước, tạo embedding sau (đúng luồng trong mockup). |
| `char_start/char_end` là offset trong văn bản **của trang** | Để frontend tô sáng đúng đoạn nguồn khi bấm trích dẫn "Trang 7". |
| `UNIQUE (document_id, chunk_index)`, CHECK cho offset/trang, `ON DELETE CASCADE` | Dữ liệu sai bị chặn ở DB, không chỉ ở code Go. Có test cho từng ràng buộc. |
| Handler → service → repository, SQL chỉ ở repository | Handler test được bằng fake, không cần DB. |
| Image runtime alpine, user không phải root, có `ca-certificates` | 30 MB; cần CA để TLS tới Neon (`sslmode=require`). |
| `ENTRYPOINT` dạng exec | Binary là PID 1 nên nhận được SIGTERM → shutdown êm. Đã đo: `docker stop` mất 1 s, exit code 0. |

---

## 2. Bug thật: khoá migration hỏng khi đi qua connection pooler (Neon)

**Bối cảnh.** Nhiều instance có thể khởi động cùng lúc (deploy, scale), và
instance nào cũng chạy migration. Bản đầu dùng `pg_advisory_lock` (khoá theo
*session*) rồi chạy mỗi file migration trong một transaction riêng.

**Vấn đề.** Trên Neon, chuỗi kết nối thường dùng là `-pooler`, tức đi qua
**PgBouncer ở transaction mode**. Ở chế độ này, mỗi câu lệnh ngoài transaction
có thể được chuyển tới một backend Postgres khác. Khoá theo session nằm trên
backend A, còn câu lệnh tiếp theo (và lệnh unlock) có thể rơi vào backend B.

**Tái hiện.** Dựng PgBouncer 1.22 local (`pool_mode = transaction`), viết test
cho 8 instance chạy `Migrate` cùng lúc trên một DB mới. Kết quả với code cũ:
**5/5 lần chạy fail**, theo hai kiểu:

1. *Khoá không loại trừ được nhau*: instance thứ hai có thể được cấp đúng backend
   đang giữ khoá, mà advisory lock thì cho phép cùng một session lấy lại (re-entrant),
   nên lấy được khoá ngay. Kết quả là nhiều instance cùng chạy `CREATE EXTENSION`,
   gây lỗi `duplicate key ... pg_extension_name_index`. Thêm một bài học phụ:
   `CREATE TABLE IF NOT EXISTS` **không an toàn khi chạy đồng thời** (lỗi
   `pg_type_typname_nsp_index`).
2. *Khoá bị kẹt lại*: lệnh unlock rơi vào backend không giữ khoá nên không có
   tác dụng, khoá trên backend kia không bao giờ được nhả, và instance khác
   chờ tới timeout 30 s.

Lưu ý: chạy trực tiếp vào Postgres (không qua pooler) thì cùng test đó **pass**.
Nếu chỉ test local là không thấy được bug này.

**Sửa.** Gom cả lần migrate vào **một transaction** và dùng
`pg_advisory_xact_lock`. PgBouncer giữ nguyên một backend suốt một transaction,
và khoá tự nhả khi commit hoặc rollback.

**Kết quả.** 10/10 lần chạy qua PgBouncer pass, 5/5 lần chạy trực tiếp pass,
và pass trên cả Go 1.22.12 lẫn 1.25.

**Đánh đổi.** Lần migrate giờ là "tất cả hoặc không": một file lỗi thì cả lần
chạy rollback (có test `TestMigrateFailureIsAllOrNothing`). Đổi lại, sau này không
dùng được `CREATE INDEX CONCURRENTLY` trong migration, vì lệnh đó không chạy được
trong transaction; khi cần sẽ phải làm thành một bước riêng.

**Bài học cho các bước sau.** Qua pooler, mọi `SET` (ví dụ `ivfflat.probes`)
phải là `SET LOCAL` bên trong transaction, nếu không thiết lập sẽ "rò" sang
request khác.

---

## 3. Đo recall của index ivfflat và lỗi trong chính phép đo

**Phát hiện.** Migration tạo index ivfflat lúc bảng còn rỗng, và Postgres cảnh
báo ngay: *"ivfflat index created with little data — This will cause low
recall."* ivfflat chia vector thành `lists` cụm dựa trên dữ liệu có **lúc tạo
index**; bảng rỗng thì tâm cụm vô nghĩa.

**Đo.** 4.000 vector 768 chiều có cấu trúc cụm (40 tâm + nhiễu, giống embedding
thật hơn dữ liệu ngẫu nhiên đều), 100 truy vấn, đo recall@10 so với quét toàn bảng.

**Lỗi trong phép đo.** Lần đo đầu cho kết quả vô lý: `probes=100` (tức quét hết
mọi cụm) mà recall chỉ được 0,27. Nguyên nhân: khi tính bảng "kết quả chính xác",
planner lại dùng chính index ivfflat, nên tôi đang so index với chính nó. Sửa
bằng `SET enable_indexscan = off` khi tính bảng chuẩn. **Bài học: luôn có một
phép kiểm tra hiển nhiên** (quét hết thì recall phải bằng 1); nhờ vậy mới bắt
được số liệu sai.

**Kết quả (recall@10):**

| Index | probes=1 (mặc định) | probes=10 | probes=100 |
|---|---|---|---|
| ivfflat tạo trên bảng **rỗng** (migration hiện tại) | **0,24** | 0,75 | 1,00 |
| ivfflat tạo **sau** khi có dữ liệu | 0,58 | 1,00 | 1,00 |
| HNSW mặc định | 1,00 | — | — |

Nghĩa là với cấu hình mặc định, khoảng 3/4 đoạn liên quan nhất sẽ không được
đưa vào ngữ cảnh cho LLM, dù code không báo lỗi gì.

**Hướng xử lý (chưa chốt):**
- Giữ ivfflat: `SET LOCAL ivfflat.probes = 10` khi truy vấn, và `REINDEX` sau khi
  đã có nhiều dữ liệu.
- Hoặc đổi sang HNSW: không phụ thuộc dữ liệu lúc tạo, recall cao ngay từ đầu;
  đổi lại tốn RAM hơn và insert chậm hơn.

Đã có test `TestCosineSearchUsesIvfflatIndex` để chặn trường hợp ai đó đổi opclass
(ví dụ sang `vector_l2_ops`): khi đó truy vấn `<=>` sẽ âm thầm bỏ qua index.

---

## 4. Các phát hiện nhỏ hơn

- **Log cấu hình không ra JSON.** Lỗi cấu hình (ví dụ `PORT=abc`) được in dạng
  text, vì `slog.SetDefault` chỉ chạy sau `config.Load()`. Sửa: bật handler JSON
  với `slog.LevelVar` trước, đọc config xong mới đặt level.
- **Build phụ thuộc mạng không cần thiết.** Dòng `# syntax=docker/dockerfile:1`
  buộc BuildKit tải frontend từ Docker Hub mỗi lần build. Khi mạng chập chờn
  (DNS timeout) build fail, dù không dùng tính năng nào cần tới nó → đã bỏ.
- **Test flaky trên macOS.** Tên DB tạm sinh bằng `time.Now().UnixNano()` bị trùng
  khi chạy song song, vì đồng hồ macOS chỉ chính xác tới micro giây → chuyển sang
  hậu tố `crypto/rand`.
- **Config báo mọi lỗi một lần** (`errors.Join`) thay vì dừng ở lỗi đầu tiên,
  để khỏi phải sửa env rồi deploy lại nhiều lần.

---

## 5. Kiểm chứng

**Tự động (41 test + subtest, pass với `-race`, trên Go 1.22.12 và 1.25):**

| Gói | Nội dung | Coverage |
|---|---|---|
| `config` | mặc định, `PORT` từ env, các lỗi cấu hình, báo nhiều lỗi cùng lúc | 97,2% |
| `service` | DB ok / lỗi / treo (timeout) | 100% |
| `handler` | 200/503 + JSON + Content-Type, 405, 404 | 96,0% |
| `repository` | migrate DB mới, chạy lại, 8 instance đồng thời, tất cả-hoặc-không, đúng thứ tự; ràng buộc schema; cascade; index cosine | 71,2% |

**Thủ công trên container:**

| Kịch bản | Kết quả |
|---|---|
| `curl /healthz` | 200 `{"status":"ok","db":"ok"}` |
| Tắt DB | 503 `{"status":"error","db":"error"}`; bật lại → 200 |
| `PORT=9123` | server nghe ở `:9123` |
| API qua PgBouncer, DB mới | migration chạy, healthz 200 |
| `docker stop` | thoát êm sau 1 s, exit code 0 |
| Host DB không tồn tại | exit 1, log JSON nêu rõ lỗi |
| Healthcheck của compose | `healthy` |

**Chưa kiểm chứng:** Neon và Render thật (pooler mới chỉ được giả lập bằng
PgBouncer 1.22; Neon pooler cũng là PgBouncer). `cmd/server` không có unit test;
phần này được kiểm bằng các kịch bản container ở trên.

---

## 6. Câu hỏi phỏng vấn có thể gặp

- **Sao chạy migration lúc khởi động mà không làm thành job riêng?** Hạ tầng
  đơn giản (Render + Neon, một service). Rủi ro chạy đồng thời đã được xử lý bằng
  advisory lock theo transaction và có test. Khi có nhiều service hoặc migration
  dài, sẽ tách thành pre-deploy job.
- **Sao healthz lại ping DB?** API này không có DB thì vô dụng, nên báo 503 để
  không nhận traffic là đúng. Đổi lại: DB chậm có thể khiến instance bị đánh dấu
  không khoẻ, nên phải có timeout ngắn.
- **pgx thay vì `database/sql`?** Hỗ trợ kiểu Postgres đầy đủ, pool tốt, nhanh
  hơn; sau này cần thêm kiểu `vector`.
- **ivfflat hay HNSW?** Xem mục 3: trả lời bằng số liệu đã đo, không phải cảm tính.

---

## Chạy lại các test

```bash
docker compose up -d db
cd api
TEST_DATABASE_URL='postgres://docinsight:docinsight@localhost:5432/docinsight?sslmode=disable' \
  go test -race ./...
```

Chạy test qua PgBouncer (giả lập Neon pooler): tạo `pgbouncer.ini` và `userlist.txt`
như dưới, rồi chạy container và đặt thêm `TEST_POOLER_URL`.

```ini
[databases]
* = host=db port=5432
[pgbouncer]
listen_addr = 0.0.0.0
listen_port = 6432
auth_type = plain
auth_file = /etc/pgbouncer/userlist.txt
pool_mode = transaction
default_pool_size = 10
max_prepared_statements = 1000
ignore_startup_parameters = extra_float_digits
```

```bash
echo '"docinsight" "docinsight"' > userlist.txt
docker run -d --name di-pgbouncer --network doc-insight_default -p 6432:6432 \
  -v "$PWD/pgbouncer.ini:/etc/pgbouncer/pgbouncer.ini:ro" \
  -v "$PWD/userlist.txt:/etc/pgbouncer/userlist.txt:ro" \
  alpine:3.20 sh -c 'apk add --no-cache pgbouncer && exec pgbouncer -u nobody /etc/pgbouncer/pgbouncer.ini'

TEST_DATABASE_URL='postgres://docinsight:docinsight@localhost:5432/docinsight?sslmode=disable' \
TEST_POOLER_URL='postgres://docinsight:docinsight@localhost:6432/docinsight?sslmode=disable' \
  go test -count=5 ./internal/repository
```
