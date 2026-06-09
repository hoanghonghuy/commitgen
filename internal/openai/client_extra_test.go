package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

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
