-- Tài liệu vừa tải lên, đang chờ worker: đổi tên trạng thái 'uploaded' -> 'pending'.
ALTER TABLE documents DROP CONSTRAINT documents_status_check;
UPDATE documents SET status = 'pending' WHERE status = 'uploaded';
ALTER TABLE documents ALTER COLUMN status SET DEFAULT 'pending';
ALTER TABLE documents ADD CONSTRAINT documents_status_check
    CHECK (status IN ('pending', 'extracting', 'chunking', 'embedding', 'ready', 'failed'));
