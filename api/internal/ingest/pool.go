// Package ingest xử lý tài liệu đã tải lên ở nền: trích văn bản PDF, cắt
// chunk, tạo embedding, lưu chunk và cập nhật trạng thái.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

var ErrQueueFull = errors.New("hàng đợi xử lý đã đầy")

// ErrDocumentDeleted: tài liệu bị xoá trong lúc đang xử lý. Không phải lỗi,
// chỉ dừng job (không còn row nào để ghi trạng thái).
var ErrDocumentDeleted = errors.New("tài liệu đã bị xoá")

type Job struct {
	DocumentID string
	Path       string // file PDF trong STORAGE_DIR
}

// Processor là phần việc chạy cho mỗi job; Fail ghi lỗi vào tài liệu.
type Processor interface {
	Process(ctx context.Context, job Job) error
	Fail(ctx context.Context, documentID, message string) error
}

// Pool chạy đúng `workers` goroutine cố định đọc job từ một channel có đệm.
// Không sinh goroutine theo từng job: số tài liệu xử lý đồng thời (và do đó
// RAM, kết nối DB, lượt gọi API embedding) luôn bị chặn trên.
type Pool struct {
	proc       Processor
	jobs       chan Job
	workers    int
	jobTimeout time.Duration
	log        *slog.Logger
	wg         sync.WaitGroup
}

func NewPool(proc Processor, workers, queueSize int, jobTimeout time.Duration) *Pool {
	return &Pool{
		proc:       proc,
		jobs:       make(chan Job, queueSize),
		workers:    workers,
		jobTimeout: jobTimeout,
		log:        slog.Default(),
	}
}

// Start khởi động các worker; chúng dừng khi ctx bị huỷ.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				// Ưu tiên dừng: select chọn ngẫu nhiên khi cả hai cùng sẵn sàng.
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case job := <-p.jobs:
					p.run(ctx, job)
				}
			}
		}()
	}
}

// Enqueue không chặn: hàng đợi đầy thì trả ErrQueueFull để request upload
// không bị treo.
func (p *Pool) Enqueue(job Job) error {
	select {
	case p.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Wait chờ mọi worker thoát (sau khi ctx của Start bị huỷ), rồi đánh dấu
// failed cho các job còn trong hàng đợi: job nằm trong bộ nhớ nên sẽ mất khi
// tiến trình dừng, để pending thì tài liệu treo mãi. Chỉ gọi sau khi đã ngừng
// nhận upload mới.
func (p *Pool) Wait() {
	p.wg.Wait()
	for {
		select {
		case job := <-p.jobs:
			p.fail(context.Background(), job.DocumentID, errors.New("server dừng trước khi kịp xử lý; hãy tải lên lại"))
		default:
			return
		}
	}
}

func (p *Pool) run(parent context.Context, job Job) {
	ctx, cancel := context.WithTimeout(parent, p.jobTimeout)
	defer cancel()
	log := p.log.With("document_id", job.DocumentID)

	start := time.Now()
	err := p.safeProcess(ctx, job)
	if err == nil {
		log.InfoContext(ctx, "ingest: done", "duration_ms", time.Since(start).Milliseconds())
		return
	}
	if errors.Is(err, ErrDocumentDeleted) {
		log.InfoContext(ctx, "ingest: document deleted during processing, job dropped")
		return
	}
	switch {
	case parent.Err() != nil:
		err = fmt.Errorf("server dừng khi đang xử lý; hãy tải lên lại: %w", err)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		err = fmt.Errorf("quá thời gian xử lý %s: %w", p.jobTimeout, err)
	}
	log.ErrorContext(ctx, "ingest: failed", "err", err, "duration_ms", time.Since(start).Milliseconds())
	p.fail(ctx, job.DocumentID, err)
}

// safeProcess đổi panic thành lỗi để một file hỏng không làm sập worker.
func (p *Pool) safeProcess(ctx context.Context, job Job) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			p.log.ErrorContext(ctx, "ingest: panic", "document_id", job.DocumentID, "panic", rec, "stack", string(debug.Stack()))
			err = fmt.Errorf("lỗi nội bộ khi xử lý: %v", rec)
		}
	}()
	return p.proc.Process(ctx, job)
}

// fail ghi lỗi bằng context riêng: ctx của job có thể đã hết hạn/bị huỷ,
// đúng lúc cần ghi lỗi nhất.
func (p *Pool) fail(ctx context.Context, documentID string, cause error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := p.proc.Fail(ctx, documentID, cause.Error()); err != nil {
		p.log.ErrorContext(ctx, "ingest: cannot record failure", "document_id", documentID, "err", err, "cause", cause)
	}
}
