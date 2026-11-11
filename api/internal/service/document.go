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
	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
)

// Lỗi nghiệp vụ; handler map sang HTTP status trong một hàm chung.
var (
	ErrDocNotFound  = errors.New("không tìm thấy tài liệu")
	ErrInvalidFile  = errors.New("file không hợp lệ")
	ErrFileTooLarge = errors.New("file vượt quá 50 MB")
	ErrBusy         = errors.New("server đang bận xử lý tài liệu khác, hãy thử lại sau")
	ErrPageNotFound = errors.New("không có trang này")
	ErrFileMissing  = errors.New("không còn file PDF gốc của tài liệu này, hãy tải lên lại")
)

const MaxUploadBytes = 50 << 20

var pdfMagic = []byte("%PDF-")

type DocumentRepo interface {
	CreateDocument(ctx context.Context, filename string, sizeBytes int64) (repository.Document, error)
	GetDocument(ctx context.Context, id string) (repository.Document, error)
	ListDocuments(ctx context.Context, limit int) ([]repository.Document, error)
	DeleteDocument(ctx context.Context, id string) error
	GetPage(ctx context.Context, documentID string, number int) (repository.Page, error)
	ListChunks(ctx context.Context, documentID string, page int) ([]repository.Chunk, error)
}

// PageView là một trang kèm các chunk nằm trên nó, đủ để trình xem tài liệu
// tô sáng đoạn nguồn theo char_start/char_end.
type PageView struct {
	DocumentID string             `json:"document_id"`
	PageCount  int                `json:"page_count"`
	Number     int                `json:"page_number"`
	Text       string             `json:"text"`
	Chunks     []repository.Chunk `json:"chunks"`
}

type JobQueue interface {
	Enqueue(job ingest.Job) error
}

type DocumentService struct {
	repo       DocumentRepo
	queue      JobQueue
	storageDir string
	maxBytes   int64
}

func NewDocumentService(repo DocumentRepo, queue JobQueue, storageDir string) *DocumentService {
	return &DocumentService{repo: repo, queue: queue, storageDir: storageDir, maxBytes: MaxUploadBytes}
}

// FilePath là nơi lưu file PDF của tài liệu id.
func (s *DocumentService) FilePath(id string) string {
	return filepath.Join(s.storageDir, id+".pdf")
}

// Upload kiểm tra file là PDF và không quá giới hạn, lưu vào STORAGE_DIR,
// tạo tài liệu status=pending rồi đưa vào hàng đợi xử lý nền.
//
// File được ghi ra file tạm trước khi tạo row, nên file sai không để lại row
// nào; nếu bước sau lỗi thì dọn cả row lẫn file.
func (s *DocumentService) Upload(ctx context.Context, filename string, r io.Reader) (repository.Document, error) {
	tmp, size, err := s.saveTemp(r)
	if err != nil {
		return repository.Document{}, err
	}
	defer os.Remove(tmp) // không còn tác dụng sau khi đã đổi tên thành công

	doc, err := s.repo.CreateDocument(ctx, cleanFilename(filename), size)
	if err != nil {
		return repository.Document{}, err
	}
	log := slog.With("document_id", doc.ID)

	path := s.FilePath(doc.ID)
	if err := os.Rename(tmp, path); err != nil {
		s.rollback(ctx, log, doc.ID, "")
		return repository.Document{}, fmt.Errorf("lưu file: %w", err)
	}
	if err := s.queue.Enqueue(ingest.Job{DocumentID: doc.ID, Path: path}); err != nil {
		s.rollback(ctx, log, doc.ID, path)
		if errors.Is(err, ingest.ErrQueueFull) {
			return repository.Document{}, ErrBusy
		}
		return repository.Document{}, fmt.Errorf("enqueue: %w", err)
	}
	log.InfoContext(ctx, "document uploaded", "filename", doc.Filename, "size_bytes", size)
	return doc, nil
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

func (s *DocumentService) List(ctx context.Context, limit int) ([]repository.Document, error) {
	return s.repo.ListDocuments(ctx, limit)
}

func (s *DocumentService) Get(ctx context.Context, id string) (repository.Document, error) {
	doc, err := s.repo.GetDocument(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return repository.Document{}, ErrDocNotFound
	}
	return doc, err
}

// readyDoc trả tài liệu đã xử lý xong; trang và chunk chỉ có sau bước đó.
func (s *DocumentService) readyDoc(ctx context.Context, id string) (repository.Document, error) {
	doc, err := s.Get(ctx, id)
	if err != nil {
		return repository.Document{}, err
	}
	if doc.Status != repository.StatusReady || doc.PageCount == nil {
		return repository.Document{}, fmt.Errorf("%w (trạng thái %s)", ErrDocNotReady, doc.Status)
	}
	return doc, nil
}

func (s *DocumentService) Page(ctx context.Context, id string, number int) (PageView, error) {
	doc, err := s.readyDoc(ctx, id)
	if err != nil {
		return PageView{}, err
	}
	if number < 1 || number > *doc.PageCount {
		return PageView{}, fmt.Errorf("%w: tài liệu có %d trang", ErrPageNotFound, *doc.PageCount)
	}
	p, err := s.repo.GetPage(ctx, doc.ID, number)
	if errors.Is(err, repository.ErrNotFound) {
		// Số trang hợp lệ mà không có dòng: tài liệu xử lý trước khi có bảng pages.
		return PageView{}, fmt.Errorf("%w: tài liệu chưa lưu văn bản trang, hãy tải lên lại", ErrPageNotFound)
	}
	if err != nil {
		return PageView{}, err
	}
	chunks, err := s.repo.ListChunks(ctx, doc.ID, number)
	if err != nil {
		return PageView{}, err
	}
	if chunks == nil {
		chunks = []repository.Chunk{} // JSON [] thay vì null
	}
	return PageView{DocumentID: doc.ID, PageCount: *doc.PageCount, Number: p.Number, Text: p.Text, Chunks: chunks}, nil
}

// Chunks trả các chunk của tài liệu; page > 0 thì chỉ lấy trang đó.
func (s *DocumentService) Chunks(ctx context.Context, id string, page int) ([]repository.Chunk, error) {
	doc, err := s.readyDoc(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.repo.ListChunks(ctx, doc.ID, page)
}

// File mở file PDF gốc để trả về cho trình duyệt; người gọi đóng file.
// Không cần tài liệu đã xử lý xong: file có từ lúc tải lên.
func (s *DocumentService) File(ctx context.Context, id string) (*os.File, repository.Document, error) {
	doc, err := s.Get(ctx, id)
	if err != nil {
		return nil, repository.Document{}, err
	}
	f, err := os.Open(s.FilePath(doc.ID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, repository.Document{}, ErrFileMissing
	}
	if err != nil {
		return nil, repository.Document{}, fmt.Errorf("open pdf %s: %w", doc.ID, err)
	}
	return f, doc, nil
}

// Delete xoá row (chunk xoá theo) rồi xoá file. Nếu worker đang xử lý tài
// liệu này, nó sẽ thấy row đã mất và tự dừng.
func (s *DocumentService) Delete(ctx context.Context, id string) error {
	if err := s.repo.DeleteDocument(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrDocNotFound
		}
		return err
	}
	log := slog.With("document_id", id)
	removeFile(ctx, log, s.FilePath(id))
	log.InfoContext(ctx, "document deleted")
	return nil
}

// removeFile không trả lỗi: row đã xoá thì request coi như thành công,
// file mồ côi chỉ cần được ghi log.
func removeFile(ctx context.Context, log *slog.Logger, path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.WarnContext(ctx, "remove file", "path", path, "err", err)
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
