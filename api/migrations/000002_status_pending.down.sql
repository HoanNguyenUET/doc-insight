ALTER TABLE documents DROP CONSTRAINT documents_status_check;
UPDATE documents SET status = 'uploaded' WHERE status = 'pending';
ALTER TABLE documents ALTER COLUMN status SET DEFAULT 'uploaded';
ALTER TABLE documents ADD CONSTRAINT documents_status_check
    CHECK (status IN ('uploaded', 'extracting', 'chunking', 'embedding', 'ready', 'failed'));
