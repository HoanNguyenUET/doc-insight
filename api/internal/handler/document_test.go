package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/HoanNguyenUET/doc-insight/api/internal/repository"
	"github.com/HoanNguyenUET/doc-insight/api/internal/service"
)

type fakeDocService struct {
	uploadErr  error
	gotName    string
	gotBody    []byte
	gotLimit   int
	docs       map[string]repository.Document
	internalEr error
	gotPage    int
}

func (f *fakeDocService) Upload(_ context.Context, filename string, r io.Reader) (repository.Document, error) {
	f.gotName = filename
	b, err := io.ReadAll(r)
	f.gotBody = b
	if err != nil {
		return repository.Document{}, fmt.Errorf("ghi file: %w", err)
	}
	if f.uploadErr != nil {
		return repository.Document{}, f.uploadErr
	}
	return repository.Document{ID: "11111111-1111-1111-1111-111111111111", Status: repository.StatusPending}, nil
}

func (f *fakeDocService) List(_ context.Context, limit int) ([]repository.Document, error) {
	f.gotLimit = limit
	if f.internalEr != nil {
		return nil, f.internalEr
	}
	var out []repository.Document
	for _, d := range f.docs {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeDocService) Get(_ context.Context, id string) (repository.Document, error) {
	d, ok := f.docs[id]
	if !ok {
		return repository.Document{}, service.ErrDocNotFound
	}
	return d, nil
}

func (f *fakeDocService) Delete(_ context.Context, id string) error {
	if _, ok := f.docs[id]; !ok {
		return service.ErrDocNotFound
	}
	delete(f.docs, id)
	return nil
}

func (f *fakeDocService) Page(_ context.Context, id string, number int) (service.PageView, error) {
	f.gotPage = number
	if _, ok := f.docs[id]; !ok {
		return service.PageView{}, service.ErrDocNotFound
	}
	if number > 2 {
		return service.PageView{}, service.ErrPageNotFound
	}
	return service.PageView{DocumentID: id, PageCount: 2, Number: number, Text: "trang", Chunks: []repository.Chunk{}}, nil
}

func (f *fakeDocService) Chunks(_ context.Context, id string, page int) ([]repository.Chunk, error) {
	f.gotPage = page
	d, ok := f.docs[id]
	if !ok {
		return nil, service.ErrDocNotFound
	}
	if d.Status != repository.StatusReady {
		return nil, service.ErrDocNotReady
	}
	return nil, nil
}

func docRouter(svc DocumentService) http.Handler {
	health := NewHealthHandler(service.NewHealthService(pingFunc(func(context.Context) error { return nil })))
	return NewRouter(Deps{Health: health, Documents: NewDocumentHandler(svc), AllowedOrigins: []string{"http://localhost:3000"}})
}

func multipartBody(t *testing.T, fields map[string]string, filename string, file []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if file != nil {
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(file)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func do(h http.Handler, req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestUploadAccepted(t *testing.T) {
	svc := &fakeDocService{}
	body, ct := multipartBody(t, map[string]string{"note": "trường khác đứng trước"}, "báo cáo.pdf", []byte("%PDF-1.7 x"))
	req := httptest.NewRequest(http.MethodPost, "/api/documents", body)
	req.Header.Set("Content-Type", ct)

	rec, got := do(docRouter(svc), req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if got["id"] != "11111111-1111-1111-1111-111111111111" || got["status"] != "pending" || len(got) != 2 {
		t.Errorf("body = %v, want {id, status:pending}", got)
	}
	if svc.gotName != "báo cáo.pdf" || string(svc.gotBody) != "%PDF-1.7 x" {
		t.Errorf("service got name %q body %q", svc.gotName, svc.gotBody)
	}
}

func TestUploadErrors(t *testing.T) {
	pdf := []byte("%PDF-1.7")
	tests := []struct {
		name       string
		ct         string
		body       io.Reader
		svcErr     error
		wantStatus int
		wantMsg    string
	}{
		{name: "not multipart", ct: "application/pdf", body: bytes.NewReader(pdf), wantStatus: 400, wantMsg: "multipart"},
		{name: "missing file field", wantStatus: 400, wantMsg: "thiếu trường file"},
		{name: "invalid file", svcErr: fmt.Errorf("%w: không phải PDF", service.ErrInvalidFile), wantStatus: 400, wantMsg: "không phải PDF"},
		{name: "too large from service", svcErr: service.ErrFileTooLarge, wantStatus: 413, wantMsg: "50 MB"},
		{name: "queue full", svcErr: service.ErrBusy, wantStatus: 503, wantMsg: "bận"},
		{name: "internal error is not leaked", svcErr: errors.New("pq: password authentication failed"), wantStatus: 500, wantMsg: "lỗi máy chủ nội bộ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, ct := tt.body, tt.ct
			if body == nil {
				var file []byte
				if tt.name != "missing file field" {
					file = pdf
				}
				b, c := multipartBody(t, map[string]string{"x": "y"}, "a.pdf", file)
				body, ct = b, c
			}
			req := httptest.NewRequest(http.MethodPost, "/api/documents", body)
			req.Header.Set("Content-Type", ct)
			rec, got := do(docRouter(&fakeDocService{uploadErr: tt.svcErr}), req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			msg, _ := got["error"].(string)
			if !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", msg, tt.wantMsg)
			}
			if strings.Contains(rec.Body.String(), "password") {
				t.Error("internal error detail leaked to client")
			}
			if got["request_id"] == "" || got["request_id"] == nil {
				t.Error("error body has no request_id")
			}
		})
	}
}

// Body vượt 50 MB + phần đầu multipart: MaxBytesReader cắt luồng và trả 413.
func TestUploadBodyOverLimit(t *testing.T) {
	big := make([]byte, service.MaxUploadBytes+multipartOverhead+10)
	copy(big, "%PDF-")
	body, ct := multipartBody(t, nil, "big.pdf", big)
	req := httptest.NewRequest(http.MethodPost, "/api/documents", body)
	req.Header.Set("Content-Type", ct)

	svc := &fakeDocService{}
	rec, got := do(docRouter(svc), req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413 (body %s)", rec.Code, rec.Body)
	}
	if got["error"] != service.ErrFileTooLarge.Error() {
		t.Errorf("error = %v", got["error"])
	}
	if len(svc.gotBody) > service.MaxUploadBytes+multipartOverhead {
		t.Errorf("service read %d bytes, more than the limit", len(svc.gotBody))
	}
}

func TestListDocuments(t *testing.T) {
	svc := &fakeDocService{}
	h := docRouter(svc)

	rec, got := do(h, httptest.NewRequest(http.MethodGet, "/api/documents", nil))
	if rec.Code != 200 || svc.gotLimit != defaultListLimit {
		t.Errorf("status %d, limit %d", rec.Code, svc.gotLimit)
	}
	if docs, ok := got["documents"].([]any); !ok || len(docs) != 0 {
		t.Errorf("empty list body = %s, want documents: []", rec.Body)
	}

	if rec, _ := do(h, httptest.NewRequest(http.MethodGet, "/api/documents?limit=200", nil)); rec.Code != 200 || svc.gotLimit != 200 {
		t.Errorf("limit=200: status %d, limit %d", rec.Code, svc.gotLimit)
	}
	for _, bad := range []string{"0", "201", "abc", "-1"} {
		if rec, _ := do(h, httptest.NewRequest(http.MethodGet, "/api/documents?limit="+bad, nil)); rec.Code != 400 {
			t.Errorf("limit=%s: status %d, want 400", bad, rec.Code)
		}
	}

	svc.internalEr = errors.New("db down")
	if rec, _ := do(h, httptest.NewRequest(http.MethodGet, "/api/documents", nil)); rec.Code != 500 {
		t.Errorf("db error: status %d, want 500", rec.Code)
	}
}

func TestGetAndDeleteDocument(t *testing.T) {
	id := "22222222-2222-2222-2222-222222222222"
	svc := &fakeDocService{docs: map[string]repository.Document{id: {ID: id, Filename: "a.pdf", Status: "ready"}}}
	h := docRouter(svc)

	rec, got := do(h, httptest.NewRequest(http.MethodGet, "/api/documents/"+id, nil))
	if rec.Code != 200 || got["id"] != id || got["filename"] != "a.pdf" || got["status"] != "ready" {
		t.Errorf("GET = %d %s", rec.Code, rec.Body)
	}
	if rec, _ := do(h, httptest.NewRequest(http.MethodDelete, "/api/documents/"+id, nil)); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("DELETE = %d %q, want 204 empty", rec.Code, rec.Body)
	}
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		rec, got := do(h, httptest.NewRequest(m, "/api/documents/"+id, nil))
		if rec.Code != 404 || got["error"] != service.ErrDocNotFound.Error() {
			t.Errorf("%s after delete = %d %s, want 404", m, rec.Code, rec.Body)
		}
	}
}

func TestPageAndChunks(t *testing.T) {
	id := "22222222-2222-2222-2222-222222222222"
	busy := "33333333-3333-3333-3333-333333333333"
	svc := &fakeDocService{docs: map[string]repository.Document{
		id:   {ID: id, Status: repository.StatusReady},
		busy: {ID: busy, Status: repository.StatusEmbedding},
	}}
	h := docRouter(svc)

	tests := []struct {
		name     string
		path     string
		want     int
		wantPage int
	}{
		{"page", "/api/documents/" + id + "/pages/2", 200, 2},
		{"page past end", "/api/documents/" + id + "/pages/3", 404, 3},
		{"page zero", "/api/documents/" + id + "/pages/0", 400, -1},
		{"page not a number", "/api/documents/" + id + "/pages/x", 400, -1},
		{"page of missing doc", "/api/documents/nope/pages/1", 404, 1},
		{"all chunks", "/api/documents/" + id + "/chunks", 200, 0},
		{"chunks of one page", "/api/documents/" + id + "/chunks?page=4", 200, 4},
		{"chunks bad page", "/api/documents/" + id + "/chunks?page=-1", 400, -1},
		{"chunks not ready", "/api/documents/" + busy + "/chunks", 409, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc.gotPage = -1
			rec, got := do(h, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want || svc.gotPage != tt.wantPage {
				t.Fatalf("status %d page %d, want %d page %d (body %s)", rec.Code, svc.gotPage, tt.want, tt.wantPage, rec.Body)
			}
			if tt.want == 200 && strings.HasSuffix(tt.path, "/2") && (got["text"] != "trang" || got["page_count"] != float64(2)) {
				t.Errorf("page body = %s", rec.Body)
			}
			if tt.want == 200 && strings.Contains(tt.path, "/chunks") {
				if c, ok := got["chunks"].([]any); !ok || len(c) != 0 {
					t.Errorf("chunks body = %s, want chunks: []", rec.Body)
				}
			}
		})
	}
}

func TestErrorStatus(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{badRequest("x"), 400},
		{fmt.Errorf("wrap: %w", service.ErrInvalidFile), 400},
		{fmt.Errorf("wrap: %w", service.ErrDocNotFound), 404},
		{fmt.Errorf("wrap: %w", service.ErrPageNotFound), 404},
		{service.ErrFileTooLarge, 413},
		{fmt.Errorf("copy: %w", &http.MaxBytesError{Limit: 1}), 413},
		{service.ErrBusy, 503},
		{errors.New("boom"), 500},
		{repository.ErrNotFound, 500}, // repository không được lọt lên handler chưa qua service
	}
	for _, tt := range tests {
		if got := errorStatus(tt.err); got != tt.want {
			t.Errorf("errorStatus(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestRequestIDHeader(t *testing.T) {
	rec, got := do(docRouter(&fakeDocService{}), httptest.NewRequest(http.MethodGet, "/api/documents/nope", nil))
	rid := rec.Header().Get(middleware.RequestIDHeader)
	if rid == "" || got["request_id"] != rid {
		t.Errorf("header request id %q, body request_id %v", rid, got["request_id"])
	}
}

func TestRecovererReturnsJSON500(t *testing.T) {
	h := middleware.RequestID(requestLogger(recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("bug")
	}))))
	rec, got := do(h, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != 500 || got["error"] != "lỗi máy chủ nội bộ" || got["request_id"] == nil {
		t.Errorf("panic response = %d %s", rec.Code, rec.Body)
	}
}

func TestCORS(t *testing.T) {
	h := docRouter(&fakeDocService{})
	tests := []struct {
		name        string
		method      string
		origin      string
		preflight   bool
		wantStatus  int
		wantAllowed bool
	}{
		{"allowed simple request", http.MethodGet, "http://localhost:3000", false, 200, true},
		{"other origin", http.MethodGet, "https://evil.example", false, 200, false},
		{"no origin", http.MethodGet, "", false, 200, false},
		{"allowed preflight", http.MethodOptions, "http://localhost:3000", true, 204, true},
		{"rejected preflight", http.MethodOptions, "https://evil.example", true, 204, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/documents", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.preflight {
				req.Header.Set("Access-Control-Request-Method", "DELETE")
			}
			rec, _ := do(h, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			allow := rec.Header().Get("Access-Control-Allow-Origin")
			if tt.wantAllowed != (allow == tt.origin && allow != "") {
				t.Errorf("Allow-Origin = %q, want allowed=%v", allow, tt.wantAllowed)
			}
			if tt.preflight && tt.wantAllowed && !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "DELETE") {
				t.Errorf("Allow-Methods = %q", rec.Header().Get("Access-Control-Allow-Methods"))
			}
			if tt.origin != "" && !strings.Contains(strings.Join(rec.Header().Values("Vary"), ","), "Origin") {
				t.Error("missing Vary: Origin")
			}
		})
	}

	star := NewRouter(Deps{Health: NewHealthHandler(service.NewHealthService(pingFunc(func(context.Context) error { return nil }))), AllowedOrigins: []string{"*"}})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://any.example")
	if rec, _ := do(star, req); rec.Header().Get("Access-Control-Allow-Origin") != "https://any.example" {
		t.Error(`"*" should allow any origin`)
	}
}
