CREATE EXTENSION IF NOT EXISTS vector;

-- Một file PDF đã tải lên. status đi theo các bước xử lý:
-- pending -> extracting -> chunking -> embedding -> ready (hoặc failed).
CREATE TABLE documents (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    filename              text        NOT NULL,
    size_bytes            bigint      NOT NULL CHECK (size_bytes >= 0),
    page_count            integer     CHECK (page_count >= 0),
    chunk_count           integer     NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
    status                text        NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'extracting', 'chunking', 'embedding', 'ready', 'failed')),
    progress              smallint    NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    error                 text,
    -- Mô hình embedding đã dùng. Vector của hai mô hình không so sánh được:
    -- khác mô hình đang chạy (hoặc NULL) thì phải xử lý lại tài liệu.
    embed_model           text,
    processing_started_at timestamptz,
    processed_at          timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX documents_created_at_idx ON documents (created_at DESC);
