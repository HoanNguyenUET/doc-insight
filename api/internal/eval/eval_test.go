package eval

import (
	"testing"
	"time"
)

func TestFold(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Điều 208", "dieu 208"},
		{"KHÔNG  đề\ncập", "khong de cap"},
		{"Hồ sơ hoàn công", "ho so hoan cong"},
	}
	for _, tt := range tests {
		if got := Fold(tt.in); got != tt.want {
			t.Errorf("Fold(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScore(t *testing.T) {
	chunks := map[int]string{0: "Điều 207. Mục kiểm tra.", 5: "Điều 208. Mục kiểm tra số 208.", 6: "Điều 213."}
	pl208 := Case{ID: "pl-208", Expect: Expect{SourceText: "Điều 208", Pages: []int{3}, AnswerAny: [][]string{{"208"}}}}
	outside := Case{ID: "out", Expect: Expect{NotInDoc: true}}

	tests := []struct {
		name     string
		c        Case
		run      Run
		wantPass bool
		wantRank int
		failed   string // tên tiêu chí phải trượt (nếu có)
	}{
		{
			name:     "đúng hết",
			c:        pl208,
			run:      Run{Sources: []Source{{0, 3}, {5, 3}}, Chunks: chunks, Answer: "Điều 208 yêu cầu lập biên bản [[2]]."},
			wantPass: true, wantRank: 2,
		},
		{
			name:     "không truy xuất được nguồn",
			c:        pl208,
			run:      Run{Sources: []Source{{0, 3}, {6, 3}}, Chunks: chunks, Answer: "Tài liệu không đề cập Điều 208."},
			wantRank: 0, failed: "truy xuất đúng nguồn",
		},
		{
			name:     "nguồn đúng nhưng ở sai trang",
			c:        pl208,
			run:      Run{Sources: []Source{{5, 2}}, Chunks: chunks, Answer: "Điều 208 [[1]]"},
			wantRank: 0, failed: "truy xuất đúng nguồn",
		},
		{
			name:     "trích dẫn trỏ nhầm đoạn",
			c:        pl208,
			run:      Run{Sources: []Source{{0, 3}, {5, 3}}, Chunks: chunks, Answer: "Điều 208 [[1]]"},
			wantRank: 2, failed: "trích dẫn trỏ đúng nguồn",
		},
		{
			name:     "trích dẫn số không có",
			c:        pl208,
			run:      Run{Sources: []Source{{5, 3}}, Chunks: chunks, Answer: "Điều 208 [[1]][[4]]"},
			wantRank: 1, failed: "trích dẫn hợp lệ",
		},
		{
			name:     "thiếu thông tin cần có",
			c:        pl208,
			run:      Run{Sources: []Source{{5, 3}}, Chunks: chunks, Answer: "Đơn vị thi công lập biên bản [[1]]."},
			wantRank: 1, failed: "có \"208\"",
		},
		{
			name:     "thông tin chỉ nằm trong phần ngoài tài liệu thì không tính",
			c:        pl208,
			run:      Run{Sources: []Source{{5, 3}}, Chunks: chunks, Answer: "Có quy định [[1]].\n\nNgoài tài liệu: Điều 208 là…"},
			wantRank: 1, failed: "có \"208\"",
		},
		{
			name:     "ngoài tài liệu: nói rõ không đề cập",
			c:        outside,
			run:      Run{Sources: []Source{{0, 1}}, Answer: "Tài liệu không đề cập.\n\nNgoài tài liệu: Paris."},
			wantPass: true,
		},
		{
			name:   "ngoài tài liệu nhưng bịa trích dẫn",
			c:      outside,
			run:    Run{Sources: []Source{{0, 1}}, Answer: "Thủ đô là Paris [[1]]."},
			failed: "nói rõ không đề cập",
		},
		{
			name:   "API lỗi",
			c:      pl208,
			run:    Run{Err: "HTTP 503", Latency: time.Second},
			failed: "không lỗi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Score(tt.c, tt.run)
			if got.Pass != tt.wantPass || got.HitRank != tt.wantRank {
				t.Errorf("Pass=%v HitRank=%d, want %v %d; checks %+v", got.Pass, got.HitRank, tt.wantPass, tt.wantRank, got.Checks)
			}
			if tt.failed != "" {
				found := false
				for _, ch := range got.Checks {
					if ch.Name == tt.failed && !ch.OK {
						found = true
					}
				}
				if !found {
					t.Errorf("check %q should fail; checks %+v", tt.failed, got.Checks)
				}
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	cases := []Case{
		{Expect: Expect{SourceText: "a"}},
		{Expect: Expect{SourceText: "b"}},
		{Expect: Expect{NotInDoc: true}},
	}
	results := []Result{
		{Pass: true, HitRank: 1, LatencyMS: 100},
		{Pass: false, HitRank: 2, LatencyMS: 200},
		{Pass: true, LatencyMS: 300},
	}
	s := Summarize(cases, results)
	if s.Passed != 2 || s.RetrievalHit != 1 || s.MRR != 0.75 || s.AvgLatencyMS != 200 {
		t.Errorf("summary = %+v", s)
	}
}
