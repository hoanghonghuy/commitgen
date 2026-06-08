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
