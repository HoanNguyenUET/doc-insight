package repository

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"net/url"
	"os"
	"testing"
)

// randomVec trả vector ngẫu nhiên (seed cố định). Vector one-hot không dùng
// được cho test ivfflat: k-means trên chúng suy biến, mọi cụm cách truy vấn
// như nhau.
func randomVec(r *rand.Rand) []float32 {
	v := make([]float32, 768)
	for i := range v {
		v[i] = float32(r.Float64() - 0.5)
	}
	return v
}

func unitAt(i int) []float32 {
	v := make([]float32, 768)
	v[i] = 1
	return v
}

func TestSearchChunks(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	doc, _ := db.CreateDocument(ctx, "a.pdf", 1)
	other, _ := db.CreateDocument(ctx, "b.pdf", 1)
	near := unitAt(1)
	near[2] = 0.5
	if err := db.CompleteDocument(ctx, doc.ID, "", blankPages(3), []NewChunk{
		{Index: 0, Content: "không", PageNumber: 1, CharStart: 0, CharEnd: 5, Embedding: unitAt(0)},
		{Index: 1, Content: "gần nhất", PageNumber: 2, CharStart: 7, CharEnd: 15, Embedding: unitAt(1)},
		{Index: 2, Content: "gần nhì", PageNumber: 3, CharStart: 3, CharEnd: 10, Embedding: near},
		{Index: 3, Content: "chưa embed", PageNumber: 3, CharStart: 10, CharEnd: 20},
	}); err != nil {
		t.Fatal(err)
	}
	// Tài liệu khác có chunk trùng hẳn vector truy vấn: không được lẫn vào.
	if err := db.CompleteDocument(ctx, other.ID, "", blankPages(1), []NewChunk{
		{Index: 0, Content: "tài liệu khác", PageNumber: 1, CharStart: 0, CharEnd: 13, Embedding: unitAt(1)},
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		limit int
		want  []string
	}{
		{"ranked by cosine", 5, []string{"gần nhất", "gần nhì", "không"}},
		{"limit", 2, []string{"gần nhất", "gần nhì"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.SearchChunks(ctx, doc.ID, unitAt(1), tt.limit)
			if err != nil {
				t.Fatalf("SearchChunks: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d matches %+v, want %v", len(got), got, tt.want)
			}
			for i, m := range got {
				if m.Content != tt.want[i] {
					t.Errorf("match %d = %q, want %q", i, m.Content, tt.want[i])
				}
			}
		})
	}

	got, _ := db.SearchChunks(ctx, doc.ID, unitAt(1), 1)
	m := got[0]
	if m.ID == 0 || m.Index != 1 || m.PageNumber != 2 || m.CharStart != 7 || m.CharEnd != 15 || math.Abs(m.Score-1) > 1e-6 {
		t.Errorf("top match = %+v, want chunk 1 on page 2 [7,15) with score 1", m)
	}

	if _, err := db.SearchChunks(ctx, "not-a-uuid", unitAt(1), 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("invalid id: err = %v, want ErrNotFound", err)
	}
	empty, _ := db.CreateDocument(ctx, "c.pdf", 1)
	if got, err := db.SearchChunks(ctx, empty.ID, unitAt(1), 5); err != nil || len(got) != 0 {
		t.Errorf("document without chunks: %v, %v; want no matches", got, err)
	}
}

// Khi bảng lớn, planner chọn quét index ivfflat cho ORDER BY khoảng cách;
// index chỉ dò một cụm (probes=1) rồi mới lọc document_id, nên truy vấn
// ngây thơ trả thiếu chunk. Dữ liệu test nhỏ thì planner không chọn index,
// nên tắt sort (enable_sort=off) để ép planner về phía ivfflat.
func TestSearchChunksNotTruncatedByIndex(t *testing.T) {
	t.Parallel()
	if os.Getenv("TEST_POOLER_URL") != "" {
		t.Skip("PgBouncer từ chối startup parameter enable_sort")
	}
	dbURL := testDatabaseURL(t)
	db := openTestDB(t, dbURL)
	ctx := context.Background()
	if err := db.Migrate(ctx, realMigrations); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	r := rand.New(rand.NewPCG(1, 2))
	for d := 0; d < 20; d++ {
		doc, _ := db.CreateDocument(ctx, "noise.pdf", 1)
		var chunks []NewChunk
		for i := 0; i < 50; i++ {
			chunks = append(chunks, NewChunk{Index: i, Content: "x", PageNumber: 1, CharStart: 0, CharEnd: 1, Embedding: randomVec(r)})
		}
		if err := db.CompleteDocument(ctx, doc.ID, "", blankPages(1), chunks); err != nil {
			t.Fatal(err)
		}
	}
	target, _ := db.CreateDocument(ctx, "target.pdf", 1)
	var chunks []NewChunk
	for i := 0; i < 8; i++ {
		chunks = append(chunks, NewChunk{Index: i, Content: "y", PageNumber: 1, CharStart: 0, CharEnd: 1, Embedding: randomVec(r)})
	}
	if err := db.CompleteDocument(ctx, target.ID, "", blankPages(1), chunks); err != nil {
		t.Fatal(err)
	}
	// Dựng lại index trên dữ liệu thật để các cụm ivfflat có ý nghĩa.
	if _, err := db.Pool.Exec(ctx, `REINDEX INDEX chunks_embedding_ivfflat_idx; ANALYZE chunks`); err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("enable_sort", "off")
	u.RawQuery = q.Encode()
	indexFirst := openTestDB(t, u.String())

	got, err := indexFirst.SearchChunks(ctx, target.ID, randomVec(r), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("got %d matches, want 5", len(got))
	}
}
