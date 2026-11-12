-- Một đoạn văn bản cắt từ tài liệu. char_start/char_end là offset rune
-- (nửa mở [start, end)) trong văn bản của trang page_number (bảng pages),
-- dùng để tô sáng đoạn nguồn khi bấm trích dẫn.
-- embedding NULL cho tới khi bước tạo embedding chạy xong.
CREATE TABLE chunks (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id  uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    chunk_index  integer     NOT NULL CHECK (chunk_index >= 0),
    content      text        NOT NULL,
    page_number  integer     NOT NULL CHECK (page_number >= 1),
    char_start   integer     NOT NULL CHECK (char_start >= 0),
    char_end     integer     NOT NULL,
    token_count  integer     CHECK (token_count >= 0),
    embedding    vector(768),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chunks_document_chunk_index_key UNIQUE (document_id, chunk_index),
    CONSTRAINT chunks_char_range_check CHECK (char_end > char_start)
);

-- Các đoạn của tài liệu X ở trang Y (tab Nội dung, nhảy trang).
CREATE INDEX chunks_document_page_idx ON chunks (document_id, page_number);

-- Tìm kiếm tương đồng cosine: ORDER BY embedding <=> $1.
-- ivfflat chia cụm theo dữ liệu có lúc tạo index; khi đã có nhiều dữ liệu
-- (vài chục nghìn đoạn) nên REINDEX và chỉnh lists ≈ số_dòng / 1000.
CREATE INDEX chunks_embedding_ivfflat_idx
    ON chunks USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
