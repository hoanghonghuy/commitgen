package openai

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func TestGenerateStream_FramingMatrix(t *testing.T) {
	original := "data: {\"choices\":[{\"delta\":{\"content\":\"feat: \"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"complete\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	for name, body := range map[string]string{
		"LF":                       original,
		"CRLF":                     strings.ReplaceAll(original, "\n", "\r\n"),
		"CR":                       strings.ReplaceAll(original, "\n", "\r"),
		"BOM":                      "\ufeff" + original,
		"no_space":                 strings.ReplaceAll(original, "data: ", "data:"),
		"comments":                 ": keepalive\n\n" + original,
		"trailing_data_whitespace": strings.Replace(original, "\n\n", "\ndata:  \n\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			var deltas strings.Builder
			content, err := New(Config{BaseURL: srv.URL, Model: "fixture"}).GenerateStream(context.Background(), sampleMsgs(), .7, func(s string) { deltas.WriteString(s) })
			if err != nil || content != "feat: complete" || deltas.String() != content {
				t.Errorf("valid SSE framing rejected or lost content: content=%q deltas=%q err=%v", content, deltas.String(), err)
			}
		})
	}
}

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

func TestBuildRequest_ZeroTemperature(t *testing.T) {
	c := New(Config{Model: "test-model"})
	for _, stream := range []bool{false, true} {
		payload, err := json.Marshal(c.buildRequest(sampleMsgs(), 0, stream))
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		var request map[string]json.RawMessage
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := string(request["temperature"]); got != "0" {
			t.Errorf("stream=%v: temperature = %q, want 0", stream, got)
		}
	}
}

func TestGenerate_GzipIntegrity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		corrupt bool
	}{
		{"valid", http.StatusOK, false},
		{"corrupt", http.StatusOK, true},
		{"client-error", http.StatusUnauthorized, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var compressed bytes.Buffer
			zw, err := gzip.NewWriterLevel(&compressed, gzip.NoCompression)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprint(zw, `{"choices":[{"message":{"content":"fix: repair cache"},"finish_reason":"stop"}]}`)
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			payload := compressed.Bytes()
			if tc.corrupt {
				index := bytes.Index(payload, []byte("repair cache"))
				if index < 0 {
					t.Fatal("fixture must contain uncompressed text")
				}
				payload[index] = 'd'
			}
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write(payload)
			}))
			defer server.Close()
			// Verify the real HTTP transport detects the damaged checksum.
			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if (readErr != nil) != tc.corrupt {
				t.Fatalf("transport control: corrupt=%v err=%v", tc.corrupt, readErr)
			}
			atomic.StoreInt32(&calls, 0)
			client := New(Config{BaseURL: server.URL, Model: "test", Timeout: time.Second})
			got, err := client.Generate(context.Background(), nil, 0)
			if tc.status != http.StatusOK {
				if err == nil || !strings.Contains(err.Error(), "401") || atomic.LoadInt32(&calls) != 1 || got != "" {
					t.Fatalf("client error lost or retried: content=%q calls=%d err=%v", got, atomic.LoadInt32(&calls), err)
				}
				return
			}
			if tc.corrupt && (err == nil || !strings.Contains(err.Error(), "checksum") || got != "") {
				t.Fatalf("provider ignored %v and accepted %q", readErr, got)
			}
			if !tc.corrupt && (err != nil || got != "fix: repair cache") {
				t.Fatalf("valid control: got=%q err=%v", got, err)
			}
		})
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
