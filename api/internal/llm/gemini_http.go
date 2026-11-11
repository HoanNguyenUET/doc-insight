package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTP: gửi request, retry/backoff và đọc lỗi của Gemini API.

type geminiErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Chỉ cần RetryInfo.retryDelay (dạng "56.9s"); các loại detail khác bỏ qua.
	Details []struct {
		RetryDelay string `json:"retryDelay"`
	} `json:"details"`
}

// post gửi body JSON tới baseURL+path, retry tối đa maxRetries lần với
// backoff luỹ thừa khi gặp 429, 5xx, lỗi mạng hoặc hết g.timeout trước khi
// có header. Thành công thì trả response đang mở
// cùng hàm huỷ timeout của lần gọi đó; người gọi đóng body rồi gọi cancel.
//
// waitOnQuota=false thì gặp 429 trả lỗi ngay thay vì chờ, để người gọi đổi
// sang model khác.
//
// API key đi qua header thay vì query string để không lộ trong log và lỗi.
func (g *GeminiProvider) post(ctx context.Context, path string, body []byte, waitOnQuota bool) (*http.Response, context.CancelFunc, error) {
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, g.timeout)
		req, err := http.NewRequestWithContext(actx, http.MethodPost, g.baseURL+path, bytes.NewReader(body))
		if err != nil {
			cancel()
			return nil, nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", g.apiKey)

		resp, err := g.client.Do(req)
		var lastErr error
		var status int
		var wait time.Duration
		switch {
		case err != nil:
			// Lỗi mạng hoặc quá g.timeout mà chưa có header: chưa đọc gì nên gọi
			// lại an toàn, trừ khi chính ctx của người gọi đã xong.
			cancel()
			if ctx.Err() != nil || attempt >= g.maxRetries {
				return nil, nil, err
			}
			lastErr, wait = err, g.backoffFor(attempt, "", 0)
		case resp.StatusCode == http.StatusOK:
			return resp, cancel, nil
		default:
			apiErr := readAPIError(resp)
			resp.Body.Close()
			cancel()
			quota := resp.StatusCode == http.StatusTooManyRequests
			retryable := (quota && waitOnQuota) || resp.StatusCode >= 500
			// Server đòi chờ quá lâu (quota theo ngày đòi hàng giờ): chờ cũng vô ích.
			if !retryable || attempt >= g.maxRetries || apiErr.RetryDelay > geminiMaxServerDelay {
				return nil, nil, apiErr
			}
			lastErr, status = apiErr, resp.StatusCode
			wait = g.backoffFor(attempt, resp.Header.Get("Retry-After"), apiErr.RetryDelay)
		}

		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait {
			return nil, nil, fmt.Errorf("%w: cần chờ %s mới gọi lại được (lỗi trước đó: %w)",
				context.DeadlineExceeded, wait.Round(time.Second), lastErr)
		}
		g.log.WarnContext(ctx, "gemini: retrying", "status", status, "err", lastErr, "attempt", attempt+1, "wait_ms", wait.Milliseconds())
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil, nil, fmt.Errorf("%w (lỗi trước đó: %w)", ctx.Err(), lastErr)
		}
	}
}

// backoffFor: backoff·2^attempt cộng jitter tới 50%, tối đa geminiMaxBackoff.
// Nếu server bảo chờ (header Retry-After tính bằng giây, hoặc RetryInfo
// trong body) lâu hơn thì chờ đúng mức đó, tối đa geminiMaxServerDelay.
func (g *GeminiProvider) backoffFor(attempt int, retryAfter string, serverDelay time.Duration) time.Duration {
	d := g.backoff << attempt
	d += time.Duration(rand.Int64N(int64(d)/2 + 1))
	d = min(d, geminiMaxBackoff)
	if s, err := strconv.Atoi(retryAfter); err == nil {
		serverDelay = max(serverDelay, time.Duration(s)*time.Second)
	}
	if serverDelay > d {
		d = min(serverDelay, geminiMaxServerDelay)
	}
	return d
}

func readAPIError(resp *http.Response) *APIError {
	// Lỗi quota kèm details khá dài; cắt ngắn thì mất RetryInfo.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body struct {
		Error geminiErrorBody `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	var delay time.Duration
	if json.Unmarshal(raw, &body) == nil {
		if body.Error.Message != "" {
			msg = body.Error.Message
		}
		for _, d := range body.Error.Details {
			if v, err := time.ParseDuration(d.RetryDelay); err == nil && v > 0 {
				delay = v
			}
		}
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg, RetryDelay: delay}
}
