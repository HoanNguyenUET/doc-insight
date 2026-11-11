package repository

import (
	"context"
	"errors"
	"testing"
)

// blankPages trả n trang rỗng cho test không quan tâm văn bản trang.
func blankPages(n int) []string {
	return make([]string, n)
}

func TestPagesAndChunks(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	ctx := context.Background()

	d, _ := db.CreateDocument(ctx, "a.pdf", 1)
	pages := []string{"Trang một.", "", "Trang ba có hai đoạn."}
	if err := db.CompleteDocument(ctx, d.ID, "", pages, []NewChunk{
		{Index: 0, Content: "Trang một.", PageNumber: 1, CharStart: 0, CharEnd: 10, TokenCount: 3},
		{Index: 1, Content: "Trang ba", PageNumber: 3, CharStart: 0, CharEnd: 8, TokenCount: 2},
		{Index: 2, Content: "có hai đoạn.", PageNumber: 3, CharStart: 9, CharEnd: 21, TokenCount: 4},
	}); err != nil {
		t.Fatal(err)
	}

	pageTests := []struct {
		name    string
		id      string
		number  int
		want    string
		wantErr error
	}{
		{"first", d.ID, 1, "Trang một.", nil},
		{"empty page kept", d.ID, 2, "", nil},
		{"unicode", d.ID, 3, "Trang ba có hai đoạn.", nil},
		{"past last page", d.ID, 4, "", ErrNotFound},
		{"bad uuid", "x", 1, "", ErrNotFound},
	}
	for _, tt := range pageTests {
		t.Run("page/"+tt.name, func(t *testing.T) {
			p, err := db.GetPage(ctx, tt.id, tt.number)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (p.Number != tt.number || p.Text != tt.want) {
				t.Errorf("page = %+v", p)
			}
		})
	}

	chunkTests := []struct {
		name string
		page int
		want []int
	}{
		{"all pages", 0, []int{0, 1, 2}},
		{"one page", 3, []int{1, 2}},
		{"page without chunks", 2, nil},
	}
	for _, tt := range chunkTests {
		t.Run("chunks/"+tt.name, func(t *testing.T) {
			got, err := db.ListChunks(ctx, d.ID, tt.page)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d chunks, want %d", len(got), len(tt.want))
			}
			for i, c := range got {
				if c.Index != tt.want[i] || c.TokenCount == nil {
					t.Errorf("chunk %d = %+v", i, c)
				}
			}
		})
	}

	// Chạy lại thay toàn bộ trang; xoá tài liệu xoá theo trang.
	if err := db.CompleteDocument(ctx, d.ID, "", []string{"Mới."}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetPage(ctx, d.ID, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("old page 3 survived rerun: %v", err)
	}
	if got := mustGet(t, db, d.ID); got.PageCount == nil || *got.PageCount != 1 {
		t.Errorf("page_count after rerun = %v", got.PageCount)
	}
	if err := db.DeleteDocument(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM pages WHERE document_id = $1`, d.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("pages after delete = %d, err %v", n, err)
	}
}
