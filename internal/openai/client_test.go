package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func sampleMsgs() []vscodeprompt.VSCodeMessage {
	return []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleSystem, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "sys"}}},
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "hi"}}},
	}
}

func TestNew_DefaultBaseURL(t *testing.T) {
	c := New(Config{Model: "gpt-4o"})
	if c.cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base URL, got %q", c.cfg.BaseURL)
	}
}

func TestGenerate_NonStreamingSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key123" {
			t.Errorf("missing/invalid auth header: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"feat: add x"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "key123", Model: "gpt-4o"})
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "feat: add x" {
		t.Errorf("got %q", got)
	}
}

func TestGenerate_StreamingSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"feat: \"}}]}\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"add y\"}}]}\n" +
			"data: [DONE]\n"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "feat: add y" {
		t.Errorf("got %q", got)
	}
}

func TestGenerate_AuthErrorNoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"auth_error"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "bad", Model: "gpt-4o"})
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain status 401, got: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("auth error must not be retried, got %d calls", got)
	}
}

func TestGenerate_NonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("expected 502 error, got: %v", err)
	}
}

func TestGenerate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.7)
	if err == nil {
		t.Error("expected error for empty choices")
	}
}

func TestTruncateString(t *testing.T) {
	if got := truncateString("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := truncateString("hello world", 5); got != "hello..." {
		t.Errorf("got %q", got)
	}
}
