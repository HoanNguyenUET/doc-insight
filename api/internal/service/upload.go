package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/HoanNguyenUET/doc-insight/api/internal/ingest"
)

// Tải tài liệu lên: kiểm tra file, lưu vào STORAGE_DIR, tạo row và đưa vào
// hàng đợi xử lý.

const MaxUploadBytes = 50 << 20

var pdfMagic = []byte("%PDF-")

// Upload kiểm tra file là PDF và không quá giới hạn, lưu vào STORAGE_DIR,
// tạo tài liệu status=pending rồi đưa vào hàng đợi xử lý nền.
//
// File được ghi ra file tạm trước khi tạo row, nên file sai không để lại row
// nào; nếu bước sau lỗi thì dọn cả row lẫn file.
func (s *DocumentService) Upload(ctx context.Context, filename string, r io.Reader) (DocumentView, error) {
	tmp, size, err := s.saveTemp(r)
	if err != nil {
		return DocumentView{}, err
	}
	defer os.Remove(tmp) // không còn tác dụng sau khi đã đổi tên thành công

	doc, err := s.repo.CreateDocument(ctx, cleanFilename(filename), size)
	if err != nil {
		return DocumentView{}, err
	}
	log := slog.With("document_id", doc.ID)

	path := s.FilePath(doc.ID)
	if err := os.Rename(tmp, path); err != nil {
		s.rollback(ctx, log, doc.ID, "")
		return DocumentView{}, fmt.Errorf("lưu file: %w", err)
	}
	if err := s.queue.Enqueue(ingest.Job{DocumentID: doc.ID, Path: path}); err != nil {
		s.rollback(ctx, log, doc.ID, path)
		if errors.Is(err, ingest.ErrQueueFull) {
			return DocumentView{}, ErrBusy
		}
		return DocumentView{}, fmt.Errorf("enqueue: %w", err)
	}
	log.InfoContext(ctx, "document uploaded", "filename", doc.Filename, "size_bytes", size)
	return s.view(doc), nil
}

func (s *DocumentService) saveTemp(r io.Reader) (path string, size int64, err error) {
	head := make([]byte, len(pdfMagic))
	if _, err := io.ReadFull(r, head); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return "", 0, fmt.Errorf("%w: file rỗng hoặc quá ngắn", ErrInvalidFile)
		}
		return "", 0, err
	}
	if !bytes.Equal(head, pdfMagic) {
		return "", 0, fmt.Errorf("%w: không phải PDF (thiếu chữ ký %%PDF-)", ErrInvalidFile)
	}

	f, err := os.CreateTemp(s.storageDir, ".upload-*.pdf")
	if err != nil {
		return "", 0, fmt.Errorf("tạo file tạm: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("ghi file: %w", cerr)
		}
		if err != nil {
			os.Remove(f.Name())
		}
	}()

	n, err := io.Copy(f, io.MultiReader(bytes.NewReader(head), io.LimitReader(r, s.maxBytes-int64(len(head))+1)))
	if err != nil {
		return "", 0, fmt.Errorf("ghi file: %w", err)
	}
	if n > s.maxBytes {
		return "", 0, ErrFileTooLarge
	}
	return f.Name(), n, nil
}

// rollback dọn tài liệu khi upload không hoàn tất. Dùng context riêng để vẫn
// dọn được khi client đã ngắt kết nối.
func (s *DocumentService) rollback(ctx context.Context, log *slog.Logger, id, path string) {
	ctx = context.WithoutCancel(ctx)
	if err := s.repo.DeleteDocument(ctx, id); err != nil {
		log.ErrorContext(ctx, "upload rollback: delete row", "err", err)
	}
	if path != "" {
		removeFile(ctx, log, path)
	}
}

// cleanFilename giữ lại tên hiển thị: bỏ đường dẫn và ký tự điều khiển,
// tối đa 255 ký tự.
func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(name, "")))
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	if name == "" || name == "." || name == "/" {
		return "document.pdf"
	}
	return name
}
