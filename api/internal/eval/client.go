package eval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client gọi API doc-insight đang chạy như một người dùng bình thường.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type document struct {
	ID       string  `json:"id"`
	Filename string  `json:"filename"`
	Status   string  `json:"status"`
	Error    *string `json:"error"`
	// NeedsReprocess: xử lý bằng mô hình khác mô hình đang chạy.
	NeedsReprocess bool `json:"needs_reprocess"`
}

// EnsureDocument trả id của tài liệu tên name; chưa có thì tải file path lên,
// còn xử lý bằng mô hình cũ thì xử lý lại, rồi chờ xử lý xong. Gọi lại nhiều
// lần không tạo bản trùng.
func (c *Client) EnsureDocument(ctx context.Context, path, name string) (string, error) {
	var list struct {
		Documents []document `json:"documents"`
	}
	if err := c.getJSON(ctx, "/api/documents?limit=200", &list); err != nil {
		return "", err
	}
	for _, d := range list.Documents {
		if d.Filename == name && d.Status != "failed" {
			if d.NeedsReprocess {
				if err := c.post(ctx, "/api/documents/"+d.ID+"/reprocess"); err != nil {
					return "", err
				}
			}
			return d.ID, c.waitReady(ctx, d.ID)
		}
	}

	id, err := c.upload(ctx, path, name)
	if err != nil {
		return "", err
	}
	return id, c.waitReady(ctx, id)
}

func (c *Client) upload(ctx context.Context, path, name string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/documents", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload %s: HTTP %d: %s", name, resp.StatusCode, b)
	}
	var d document
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return "", fmt.Errorf("upload %s: %w", name, err)
	}
	return d.ID, nil
}

func (c *Client) waitReady(ctx context.Context, id string) error {
	for {
		var d document
		if err := c.getJSON(ctx, "/api/documents/"+id, &d); err != nil {
			return err
		}
		switch d.Status {
		case "ready":
			return nil
		case "failed":
			msg := ""
			if d.Error != nil {
				msg = *d.Error
			}
			return fmt.Errorf("tài liệu %s xử lý lỗi: %s", id, msg)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Chunks trả nội dung từng đoạn của tài liệu theo chỉ số đoạn.
func (c *Client) Chunks(ctx context.Context, id string) (map[int]string, error) {
	var out struct {
		Chunks []struct {
			Index   int    `json:"index"`
			Content string `json:"content"`
		} `json:"chunks"`
	}
	if err := c.getJSON(ctx, "/api/documents/"+id+"/chunks", &out); err != nil {
		return nil, err
	}
	m := make(map[int]string, len(out.Chunks))
	for _, ch := range out.Chunks {
		m[ch.Index] = ch.Content
	}
	return m, nil
}

// Ask hỏi qua SSE và gom kết quả. Lỗi của API (HTTP hay event error) nằm
// trong Run.Err chứ không trả về, để một câu lỗi không dừng cả bộ.
func (c *Client) Ask(ctx context.Context, id, question string) (run Run) {
	start := time.Now()
	// run là kết quả có tên để defer gán được độ trễ vào giá trị trả về.
	defer func() { run.Latency = time.Since(start) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api/documents/"+id+"/ask?q="+url.QueryEscape(question), nil)
	if err != nil {
		run.Err = err.Error()
		return run
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		run.Err = err.Error()
		return run
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		run.Err = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		return run
	}

	var answer strings.Builder
	var event string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			event = name
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		switch event {
		case "sources":
			var s struct {
				Chunks []Source `json:"chunks"`
			}
			_ = json.Unmarshal([]byte(data), &s)
			run.Sources = s.Chunks
		case "token":
			var tok struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal([]byte(data), &tok)
			answer.WriteString(tok.Text)
		case "error":
			var e struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal([]byte(data), &e)
			run.Err = e.Message
		}
	}
	if err := sc.Err(); err != nil && run.Err == "" {
		run.Err = err.Error()
	}
	run.Answer = answer.String()
	return run
}

func (c *Client) post(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: HTTP %d: %s", path, resp.StatusCode, b)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
