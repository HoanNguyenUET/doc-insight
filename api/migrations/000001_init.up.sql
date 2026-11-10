CREATE EXTENSION IF NOT EXISTS vector;

-- Một file PDF đã tải lên. status đi theo các bước xử lý:
-- uploaded -> extracting -> chunking -> embedding -> ready (hoặc failed).
CREATE TABLE documents (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    filename              text        NOT NULL,
    size_bytes            bigint      NOT NULL CHECK (size_bytes >= 0),
    page_count            integer     CHECK (page_count >= 0),
    chunk_count           integer     NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    status                text        NOT NULL DEFAULT 'uploaded'
                          CHECK (status IN ('uploaded', 'extracting', 'chunking', 'embedding', 'ready', 'failed')),
    progress              smallint    NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    error                 text,
    embedding_model       text,
    processing_started_at timestamptz,
    processed_at          timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX documents_created_at_idx ON documents (created_at DESC);

-- Một đoạn văn bản cắt từ tài liệu. char_start/char_end là offset ký tự
-- (nửa mở [start, end)) trong văn bản trích xuất của trang page_number,
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

-- Phục vụ truy vấn "các đoạn của tài liệu X ở trang Y" (màn Chỉ mục, nhảy trang).
CREATE INDEX chunks_document_page_idx ON chunks (document_id, page_number);

-- Tìm kiếm tương đồng cosine: ORDER BY embedding <=> $1.
-- ivfflat chia cụm theo dữ liệu có lúc tạo index; khi đã có nhiều dữ liệu
-- (vài chục nghìn đoạn) nên REINDEX và chỉnh lists ≈ số_dòng / 1000.
CREATE INDEX chunks_embedding_ivfflat_idx
    ON chunks USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
