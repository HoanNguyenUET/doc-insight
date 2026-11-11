package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProc chạy hàm process tuỳ biến và ghi lại các lần Fail.
type fakeProc struct {
	process func(ctx context.Context, job Job) error

	mu    sync.Mutex
	fails map[string]string
	done  chan string // nhận document_id khi job kết thúc (thành công hoặc Fail)
}

func newFakeProc(process func(ctx context.Context, job Job) error) *fakeProc {
	return &fakeProc{process: process, fails: map[string]string{}, done: make(chan string, 100)}
}

func (f *fakeProc) Process(ctx context.Context, job Job) error {
	err := f.process(ctx, job)
	if err == nil {
		f.done <- job.DocumentID
	}
	return err
}

func (f *fakeProc) Fail(ctx context.Context, id, msg string) error {
	if ctx.Err() != nil {
		return ctx.Err() // pool phải dùng context còn sống để ghi lỗi
	}
	f.mu.Lock()
	f.fails[id] = msg
	f.mu.Unlock()
	f.done <- id
	return nil
}

func (f *fakeProc) failMsg(id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.fails[id]
	return m, ok
}

func waitDone(t *testing.T, f *fakeProc, n int) []string {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		select {
		case id := <-f.done:
			ids = append(ids, id)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out after %d/%d jobs", i, n)
		}
	}
	return ids
}

func startPool(t *testing.T, proc Processor, workers, queue int, timeout time.Duration) (*Pool, context.CancelFunc) {
	t.Helper()
	p := NewPool(proc, workers, queue, timeout)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	t.Cleanup(func() { cancel(); p.Wait() })
	return p, cancel
}

func TestPoolLimitsConcurrency(t *testing.T) {
	var running, peak atomic.Int32
	release := make(chan struct{})
	proc := newFakeProc(func(ctx context.Context, job Job) error {
		n := running.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		<-release
		running.Add(-1)
		return nil
	})
	p, _ := startPool(t, proc, 4, 20, time.Minute)

	for i := 0; i < 12; i++ {
		if err := p.Enqueue(Job{DocumentID: fmt.Sprint(i)}); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}
	// Chờ cho tới khi đủ 4 worker cùng bận rồi giữ thêm một lúc: không được vượt 4.
	deadline := time.Now().Add(2 * time.Second)
	for running.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := running.Load(); got != 4 {
		t.Errorf("running = %d, want 4", got)
	}
	close(release)
	waitDone(t, proc, 12)
	if got := peak.Load(); got != 4 {
		t.Errorf("peak concurrency = %d, want 4", got)
	}
}

func TestPoolQueueFull(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	proc := newFakeProc(func(ctx context.Context, job Job) error { <-block; return nil })
	p, _ := startPool(t, proc, 1, 2, time.Minute)

	// 1 job đang chạy + 2 job trong hàng đợi; job tiếp theo phải bị từ chối ngay.
	var accepted int
	for i := 0; i < 10; i++ {
		if err := p.Enqueue(Job{DocumentID: fmt.Sprint(i)}); err != nil {
			if !errors.Is(err, ErrQueueFull) {
				t.Fatalf("err = %v", err)
			}
			break
		}
		accepted++
		time.Sleep(10 * time.Millisecond)
	}
	if accepted != 3 {
		t.Errorf("accepted %d jobs, want 3 (1 running + queue 2)", accepted)
	}
}

func TestPoolJobTimeout(t *testing.T) {
	proc := newFakeProc(func(ctx context.Context, job Job) error {
		<-ctx.Done()
		return ctx.Err()
	})
	p, _ := startPool(t, proc, 1, 1, 50*time.Millisecond)

	start := time.Now()
	_ = p.Enqueue(Job{DocumentID: "slow"})
	waitDone(t, proc, 1)
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("job ran %v, want it cut at the timeout", el)
	}
	msg, ok := proc.failMsg("slow")
	if !ok || !strings.Contains(msg, "quá thời gian xử lý 50ms") {
		t.Errorf("fail message = %q, want timeout message", msg)
	}
}

func TestPoolRecordsErrorAndSurvivesPanic(t *testing.T) {
	proc := newFakeProc(func(ctx context.Context, job Job) error {
		switch job.DocumentID {
		case "boom":
			var m map[string]int
			m["x"] = 1 // panic: assignment to entry in nil map
		case "bad":
			return errors.New("PDF hỏng")
		}
		return nil
	})
	p, _ := startPool(t, proc, 1, 10, time.Minute)

	for _, id := range []string{"boom", "bad", "ok"} {
		_ = p.Enqueue(Job{DocumentID: id})
	}
	waitDone(t, proc, 3) // worker duy nhất vẫn chạy tiếp sau panic

	if msg, _ := proc.failMsg("boom"); !strings.Contains(msg, "nil map") {
		t.Errorf("panic fail message = %q", msg)
	}
	if msg, _ := proc.failMsg("bad"); msg != "PDF hỏng" {
		t.Errorf("error fail message = %q", msg)
	}
	if _, failed := proc.failMsg("ok"); failed {
		t.Error("successful job marked failed")
	}
}

func TestPoolDeletedDocumentIsNotFailed(t *testing.T) {
	proc := newFakeProc(func(ctx context.Context, job Job) error { return fmt.Errorf("x: %w", ErrDocumentDeleted) })
	p, _ := startPool(t, proc, 1, 1, time.Minute)
	_ = p.Enqueue(Job{DocumentID: "gone"})
	_ = p.Enqueue(Job{DocumentID: "gone2"})
	time.Sleep(100 * time.Millisecond)
	if _, failed := proc.failMsg("gone"); failed {
		t.Error("deleted document should not be marked failed")
	}
}

// Khi dừng server: job đang chạy bị huỷ và ghi failed, job còn trong hàng
// đợi cũng ghi failed (không để pending mãi), và Wait trả về.
func TestPoolShutdown(t *testing.T) {
	started := make(chan struct{})
	proc := newFakeProc(func(ctx context.Context, job Job) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	p := NewPool(proc, 1, 10, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	_ = p.Enqueue(Job{DocumentID: "running"})
	<-started
	_ = p.Enqueue(Job{DocumentID: "queued1"})
	_ = p.Enqueue(Job{DocumentID: "queued2"})

	cancel()
	waited := make(chan struct{})
	go func() { p.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return")
	}

	if msg, _ := proc.failMsg("running"); !strings.Contains(msg, "server dừng khi đang xử lý") {
		t.Errorf("running job fail message = %q", msg)
	}
	for _, id := range []string{"queued1", "queued2"} {
		if msg, _ := proc.failMsg(id); !strings.Contains(msg, "server dừng trước khi kịp xử lý") {
			t.Errorf("%s fail message = %q", id, msg)
		}
	}
}
