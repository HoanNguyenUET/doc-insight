-- Văn bản đã chuẩn hoá của từng trang, đúng chuỗi mà chunker đã cắt:
-- char_start/char_end của chunk là offset rune trên cột text này. Trình xem
-- tài liệu hiển thị text và tô sáng đoạn nguồn theo offset.
-- Tài liệu xử lý trước migration này không có dòng nào; phải tải lên lại.
CREATE TABLE pages (
    document_id uuid    NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    page_number integer NOT NULL CHECK (page_number >= 1),
    text        text    NOT NULL,
    PRIMARY KEY (document_id, page_number)
);
