package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRPCStreamDeliversLiveNotifications(t *testing.T) {
	t.Setenv("AIM_ENV", "test")
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	s := &Server{service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc/stream", s.HandleRPCStream)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	respCh, errCh := openRPCStreamAsync(t, ts.URL, 0)
	evt := svc.PublishNotificationForTesting("notify.test", map[string]any{"x": 1})
	resp := awaitRPCStreamResponse(t, respCh, errCh)
	defer closeResponseBody(t, resp)
	got := readRPCStreamEvent(t, resp.Body)
	var notification struct {
		Method string `json:"method"`
		Params struct {
			Version int            `json:"version"`
			Seq     int64          `json:"seq"`
			Payload map[string]any `json:"payload"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(got), &notification); err != nil {
		t.Fatalf("decode notification failed: %v", err)
	}
	if notification.Method != "notify.test" {
		t.Fatalf("unexpected method: %s", notification.Method)
	}
	if notification.Params.Seq != evt.Seq {
		t.Fatalf("unexpected seq: %d", notification.Params.Seq)
	}
	if notification.Params.Version != 1 {
		t.Fatalf("unexpected version: %d", notification.Params.Version)
	}
}

func TestRPCStreamReplaysFromCursor(t *testing.T) {
	t.Setenv("AIM_ENV", "test")
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	first := svc.PublishNotificationForTesting("notify.first", map[string]any{"m": "1"})
	second := svc.PublishNotificationForTesting("notify.second", map[string]any{"m": "2"})

	s := &Server{service: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc/stream", s.HandleRPCStream)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp := openRPCStream(t, ts.URL, first.Seq)
	defer closeResponseBody(t, resp)

	got := readRPCStreamEvent(t, resp.Body)
	var notification struct {
		Method string `json:"method"`
		Params struct {
			Seq int64 `json:"seq"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(got), &notification); err != nil {
		t.Fatalf("decode notification failed: %v", err)
	}
	if notification.Method != "notify.second" {
		t.Fatalf("expected notify.second, got %s", notification.Method)
	}
	if notification.Params.Seq != second.Seq {
		t.Fatalf("expected seq %d, got %d", second.Seq, notification.Params.Seq)
	}
}

func openRPCStream(t *testing.T, baseURL string, cursor int64) *http.Response {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/rpc/stream?cursor="+strconv.FormatInt(cursor, 10), nil)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	return resp
}

func openRPCStreamAsync(t *testing.T, baseURL string, cursor int64) (<-chan *http.Response, <-chan error) {
	t.Helper()
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/rpc/stream?cursor="+strconv.FormatInt(cursor, 10), nil)
	if err != nil {
		errCh <- err
		return respCh, errCh
	}

	go func() {
		resp, reqErr := http.DefaultClient.Do(req)
		if reqErr != nil {
			errCh <- reqErr
			return
		}
		respCh <- resp
	}()
	return respCh, errCh
}

func awaitRPCStreamResponse(t *testing.T, respCh <-chan *http.Response, errCh <-chan error) *http.Response {
	t.Helper()
	select {
	case err := <-errCh:
		t.Fatalf("stream request failed: %v", err)
		return nil
	case resp := <-respCh:
		return resp
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream response")
		return nil
	}
}

func closeResponseBody(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("close response body failed: %v", err)
	}
}

func readRPCStreamEvent(t *testing.T, body io.ReadCloser) string {
	t.Helper()
	got, err := readSSEDataLine(body, 2*time.Second)
	if err != nil {
		t.Fatalf("read sse failed: %v", err)
	}
	return got
}

func readSSEDataLine(body io.ReadCloser, timeout time.Duration) (string, error) {
	result := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				result <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
			return
		}
		errCh <- context.Canceled
	}()
	select {
	case out := <-result:
		return out, nil
	case err := <-errCh:
		return "", err
	case <-time.After(timeout):
		return "", context.DeadlineExceeded
	}
}
