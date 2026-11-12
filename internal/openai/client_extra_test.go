package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGenerate_RejectsIncompleteCompletion(t *testing.T) {
	for _, reason := range []string{"length", "content_filter", "tool_calls"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", reason, stream), func(t *testing.T) {
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if stream {
						fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"feat: partial\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":%q}]}\n\ndata: [DONE]\n\n", reason)
					} else {
						fmt.Fprintf(w, `{"choices":[{"message":{"content":"feat: partial"},"finish_reason":%q}]}`, reason)
					}
				}))
				defer srv.Close()
				client := New(Config{BaseURL: srv.URL, Model: "fixture"})
				var got string
				var err error
				var deltas strings.Builder
				if stream {
					got, err = client.GenerateStream(context.Background(), sampleMsgs(), .7, func(s string) { deltas.WriteString(s) })
				} else {
					got, err = client.Generate(context.Background(), sampleMsgs(), .7)
				}
				if err == nil || !strings.Contains(err.Error(), reason) || got != "" {
					t.Errorf("incomplete completion accepted: text=%q err=%v", got, err)
				}
				if calls.Load() != 1 || (stream && deltas.String() != "feat: partial") {
					t.Errorf("calls=%d partial display=%q", calls.Load(), deltas.String())
				}
			})
		}
	}
}

func TestStreamingResponse_MultilineData(t *testing.T) {
	for _, stream := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": keepalive\n\ndata: {\"choices\":[\ndata: {\"delta\":{\"content\":\"feat: complete event\"},\"finish_reason\":\"stop\"}]}\n\ndata:[DONE]\n\n")
		}))
		c := New(Config{BaseURL: srv.URL, Model: "fixture"})
		var got string
		var err error
		if stream {
			got, err = c.GenerateStream(context.Background(), sampleMsgs(), 0.7, nil)
		} else {
			got, err = c.generate(context.Background(), sampleMsgs(), 0.7)
		}
		srv.Close()
		if err != nil || got != "feat: complete event" {
			t.Errorf("stream=%v: content=%q err=%v", stream, got, err)
		}
	}
}

func TestGenerate_CancelsRetryBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := New(Config{BaseURL: srv.URL, Model: "fixture"}).Generate(ctx, sampleMsgs(), 0.7)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 750*time.Millisecond {
		t.Errorf("retry ignored deadline: elapsed=%s err=%v", time.Since(started), err)
	}
}

func TestStreamingResponse_OptionalDataSpace(t *testing.T) {
	for _, prefix := range []string{"data:", "data: "} {
		for _, header := range []string{"", "text/event-stream; charset=utf-8"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("prefix=%q/header=%q/stream=%v", prefix, header, stream), func(t *testing.T) {
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if header != "" {
							w.Header().Set("Content-Type", header)
						}
						if header != "" {
							fmt.Fprint(w, ": keepalive\n\nevent: message\n")
						}
						fmt.Fprintf(w, "%s{\"choices\":[{\"delta\":{\"content\":\"feat: valid SSE\"}}]}\n\n%s[DONE]\n\n", prefix, prefix)
					}))
					defer srv.Close()
					c := New(Config{BaseURL: srv.URL, Model: "fixture"})
					var deltas strings.Builder
					var got string
					var err error
					if stream {
						got, err = c.GenerateStream(context.Background(), sampleMsgs(), 0.7, func(s string) { deltas.WriteString(s) })
					} else {
						got, err = c.generate(context.Background(), sampleMsgs(), 0.7)
					}
					if err != nil || got != "feat: valid SSE" {
						t.Fatalf("valid SSE rejected: content=%q err=%v", got, err)
					}
					if stream && deltas.String() != got {
						t.Errorf("callback content=%q want=%q", deltas.String(), got)
					}
				})
			}
		}
	}
}

func TestStreamingResponse_RequiresCompletion(t *testing.T) {
	for _, terminal := range []string{"", "data: [DONE]\n\n", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("terminal=%q/stream=%v", terminal, stream), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"feat: partial\"},\"finish_reason\":null}]}\n\n"+terminal)
				}))
				defer srv.Close()
				c := New(Config{BaseURL: srv.URL, Model: "fixture"})
				var got string
				var err error
				if stream {
					got, err = c.GenerateStream(context.Background(), sampleMsgs(), 0.7, nil)
				} else {
					got, err = c.generate(context.Background(), sampleMsgs(), 0.7)
				}
				if terminal == "" {
					if !errors.Is(err, io.ErrUnexpectedEOF) || got != "" {
						t.Errorf("incomplete stream accepted: content=%q err=%v", got, err)
					}
				} else if err != nil || got != "feat: partial" {
					t.Errorf("complete stream rejected: content=%q err=%v", got, err)
				}
			})
		}
	}
}

func TestGenerate_EmptyStreamingResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SSE prefix but no content, only DONE marker.
		_, _ = w.Write([]byte("data: [DONE]\n"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err == nil {
		t.Error("expected error for empty streaming response")
	}
}

func TestGenerate_StreamingMessageFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Some providers stream using message.content instead of delta.content.
		_, _ = w.Write([]byte("data: {\"choices\":[{\"message\":{\"content\":\"feat: z\"}}]}\n" +
			"data: [DONE]\n"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "feat: z" {
		t.Errorf("got %q", got)
	}
}

func TestGenerate_RetryThenSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First attempt: transient server error (retryable).
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"temporary","type":"server_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: recovered"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if got != "feat: recovered" {
		t.Errorf("got %q", got)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Errorf("expected at least 2 calls (1 retry), got %d", calls)
	}
}

func TestGenerate_ClientErrorNoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// 400 is a client error (not 429) → must not be retried.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err == nil {
		t.Fatal("expected client error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("client error (400) must not be retried, got %d calls", got)
	}
}

func TestGenerate_RateLimitRetried(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// 429 is retryable.
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: ok"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err != nil {
		t.Fatalf("expected success after 429 retry, got: %v", err)
	}
	if got != "feat: ok" {
		t.Errorf("got %q", got)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Errorf("expected at least 2 calls (429 retried), got %d", calls)
	}
}

func TestGenerateStream_CollectsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"feat: \"}}]}\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"add stream\"}}]}\n" +
			"data: [DONE]\n"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	var deltas []string
	full, err := c.GenerateStream(context.Background(), sampleMsgs(), 0.7, func(d string) {
		deltas = append(deltas, d)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if full != "feat: add stream" {
		t.Errorf("full = %q", full)
	}
	if len(deltas) != 2 {
		t.Errorf("expected 2 deltas, got %d: %v", len(deltas), deltas)
	}
}

func TestStreamingResponse_RejectsMidstreamError(t *testing.T) {
	for _, failure := range []string{
		`{"error":{"message":"Provider failed","type":"server_error"},"choices":[{"delta":{},"finish_reason":"error"}]}`,
		`{"choices":[{"delta":{},"finish_reason":"error"}]}`,
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", failure, stream), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"feat: partial\"}}]}\n\ndata: %s\n\ndata: [DONE]\n\n", failure)
				}))
				defer srv.Close()
				c := New(Config{BaseURL: srv.URL, Model: "fixture"})
				var text string
				var err error
				if stream {
					text, err = c.GenerateStream(context.Background(), sampleMsgs(), 0.7, nil)
				} else {
					text, err = c.generate(context.Background(), sampleMsgs(), 0.7)
				}
				if err == nil || text != "" {
					t.Fatalf("midstream error accepted: content=%q err=%v", text, err)
				}
			})
		}
	}
}

func TestGenerateStream_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "bad", Model: "gpt-4o"})
	_, err := c.GenerateStream(context.Background(), sampleMsgs(), 0.7, nil)
	if err == nil {
		t.Error("expected error for non-200 stream response")
	}
}
