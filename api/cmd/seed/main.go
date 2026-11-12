// Lệnh seed tải các PDF mẫu lên API đang chạy, như người dùng thật, rồi đợi
// xử lý xong, để có sẵn dữ liệu demo:
//
//	go run ./cmd/seed -api http://localhost:8080
//
// Chạy lại nhiều lần không tạo bản trùng (tài liệu cùng tên được dùng lại;
// tài liệu xử lý bằng mô hình cũ thì được xử lý lại).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HoanNguyenUET/doc-insight/api/internal/eval"
)

func main() {
	api := flag.String("api", "http://localhost:8080", "địa chỉ API")
	dir := flag.String("docs", "eval/docs", "thư mục chứa PDF mẫu")
	flag.Parse()

	if err := run(*api, *dir); err != nil {
		slog.Error("seed", "err", err)
		os.Exit(1)
	}
}

func run(api, dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	files, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("không có file PDF nào trong %s", dir)
	}
	client := &eval.Client{BaseURL: strings.TrimRight(api, "/"), HTTP: &http.Client{Timeout: time.Minute}}
	for _, f := range files {
		start := time.Now()
		id, err := client.EnsureDocument(ctx, f, filepath.Base(f))
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		fmt.Printf("✓ %-28s %s  (%s)\n", filepath.Base(f), id, time.Since(start).Round(100*time.Millisecond))
	}
	fmt.Printf("\nĐã seed %d tài liệu. Mở http://localhost:3000 để xem.\n", len(files))
	return nil
}
