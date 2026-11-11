// Lệnh eval chấm chất lượng câu trả lời trên bộ câu hỏi chuẩn (eval/cases.json)
// bằng cách gọi API đang chạy:
//
//	go run ./cmd/eval -api http://localhost:8080
//
// Mỗi câu hỏi tốn một lượt gọi model trả lời; tài liệu mẫu được tải lên một
// lần (tên "eval-...") rồi dùng lại. Thoát với mã 1 nếu tỉ lệ đạt dưới -min.
package main

import (
	"context"
	"encoding/json"
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
	casesPath := flag.String("cases", "eval/cases.json", "file câu hỏi")
	docsDir := flag.String("docs", "eval/docs", "thư mục PDF mẫu")
	only := flag.String("only", "", "chỉ chạy các câu có id chứa chuỗi này")
	out := flag.String("out", "", "ghi kết quả chi tiết ra file JSON")
	minPass := flag.Float64("min", 0, "tỉ lệ đạt tối thiểu (0-1); thấp hơn thì thoát mã 1")
	flag.Parse()

	if err := run(*api, *casesPath, *docsDir, *only, *out, *minPass); err != nil {
		slog.Error("eval", "err", err)
		os.Exit(1)
	}
}

func run(api, casesPath, docsDir, only, out string, minPass float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	raw, err := os.ReadFile(casesPath)
	if err != nil {
		return fmt.Errorf("đọc %s: %w", casesPath, err)
	}
	var all []eval.Case
	if err := json.Unmarshal(raw, &all); err != nil {
		return fmt.Errorf("đọc %s: %w", casesPath, err)
	}
	var cases []eval.Case
	for _, c := range all {
		if only == "" || strings.Contains(c.ID, only) {
			cases = append(cases, c)
		}
	}

	client := &eval.Client{BaseURL: strings.TrimRight(api, "/"), HTTP: &http.Client{Timeout: 3 * time.Minute}}
	docIDs := map[string]string{}
	chunks := map[string]map[int]string{}
	results := make([]eval.Result, 0, len(cases))

	for _, c := range cases {
		if _, ok := docIDs[c.Doc]; !ok {
			id, err := client.EnsureDocument(ctx, filepath.Join(docsDir, c.Doc))
			if err != nil {
				return err
			}
			docIDs[c.Doc] = id
			if chunks[c.Doc], err = client.Chunks(ctx, id); err != nil {
				return err
			}
		}
		r := client.Ask(ctx, docIDs[c.Doc], c.Question)
		r.Chunks = chunks[c.Doc]
		res := eval.Score(c, r)
		results = append(results, res)
		printResult(res)
	}

	s := eval.Summarize(cases, results)
	fmt.Printf("\nĐạt %d/%d (%.0f%%) · nguồn đúng trong top-K %.0f%% · MRR %.2f · trung bình %d ms\n",
		s.Passed, s.Cases, s.PassRate*100, s.RetrievalHit*100, s.MRR, s.AvgLatencyMS)

	if out != "" {
		b, _ := json.MarshalIndent(map[string]any{"summary": s, "results": results}, "", "  ")
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return fmt.Errorf("ghi %s: %w", out, err)
		}
	}
	if s.PassRate < minPass {
		return fmt.Errorf("tỉ lệ đạt %.0f%% dưới mức %.0f%%", s.PassRate*100, minPass*100)
	}
	return nil
}

func printResult(r eval.Result) {
	mark := "ĐẠT "
	if !r.Pass {
		mark = "TRƯỢT"
	}
	rank := "-"
	if r.HitRank > 0 {
		rank = fmt.Sprint(r.HitRank)
	}
	fmt.Printf("%s %-14s nguồn #%s %5dms  %s\n", mark, r.ID, rank, r.LatencyMS, r.Question)
	for _, ch := range r.Checks {
		if !ch.OK {
			fmt.Printf("      ✗ %s %s\n", ch.Name, ch.Note)
		}
	}
	if !r.Pass && r.Answer != "" {
		a := strings.ReplaceAll(r.Answer, "\n", " ")
		if len([]rune(a)) > 160 {
			a = string([]rune(a)[:160]) + "…"
		}
		fmt.Printf("      trả lời: %s\n", a)
	}
}
