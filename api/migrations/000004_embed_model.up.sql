-- Mô hình embedding đã dùng khi xử lý tài liệu. Vector của hai mô hình khác
-- nhau không so sánh được: đổi mô hình thì tài liệu cũ phải xử lý lại.
-- NULL = tài liệu xử lý trước khi có cột này (không rõ mô hình).
ALTER TABLE documents ADD COLUMN embed_model text;
