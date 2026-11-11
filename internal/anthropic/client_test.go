package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func TestGenerateStream_RejectsMidstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"feat: partial\"}}\n\nevent: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"))
	}))
	defer srv.Close()
	text, err := newTestClient(srv.URL).GenerateStream(context.Background(), sampleMsgs(), 0.7, nil)
	if err == nil || !strings.Contains(err.Error(), "Overloaded") {
		t.Fatalf("midstream error accepted: content=%q err=%v", text, err)
	}
}

func TestGenerate_PreservesAllTextBlocks(t *testing.T) {
	for _, prefix := range []string{"", `{"type":"thinking","thinking":"PRIVATE_THOUGHT"},`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"content":[` + prefix + `{"type":"text","text":"feat: complete"},{"type":"text","text":"\n\nBody and footer"}]}`))
		}))
		got, err := newTestClient(srv.URL).Generate(context.Background(), sampleMsgs(), 0.7)
		srv.Close()
		if err != nil || got != "feat: complete\n\nBody and footer" {
			t.Errorf("text blocks lost: got=%q err=%v", got, err)
		}
	}
}

func sampleMsgs() []vscodeprompt.VSCodeMessage {
	return []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleSystem, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "system prompt"}}},
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "user text"}}},
	}
}

// newTestClient builds a client pointed at a test server.
func newTestClient(baseURL string) *Client {
	return &Client{
		apiKey:  "test-key",
		model:   "claude-3-opus",
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func TestNew_SetsDefaultURL(t *testing.T) {
	c := New(Config{APIKey: "k", Model: "m"})
	if c.baseURL != defaultAnthropicURL {
		t.Errorf("expected default URL, got %q", c.baseURL)
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

func TestGenerate_SystemPromptAndTemperature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic-version header")
		}
		var req messageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if req.System != "system prompt" {
			t.Errorf("system prompt not extracted: %q", req.System)
		}
		if req.Temperature != 0.3 {
			t.Errorf("temperature not propagated: %v", req.Temperature)
		}
		if req.MaxTokens != anthropicMaxTokens {
			t.Errorf("max tokens mismatch: %d", req.MaxTokens)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("messages mapping wrong: %+v", req.Messages)
		}
		_, _ = w.Write([]byte(`{"content":[{"text":"fix: bug"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fix: bug" {
		t.Errorf("got %q", got)
	}
}

func TestGenerate_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.3)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestGenerate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.3)
	if err == nil {
		t.Error("expected error for 429 response")
	}
}

func TestGenerateStream_CollectsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header: %q", r.Header.Get("x-api-key"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"feat: \"}}\n\n" +
				"event: content_block_delta\n" +
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"add stream\"}}\n\n" +
				"event: message_stop\n" +
				"data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	c := &Client{apiKey: "test-key", model: "claude-3-opus", baseURL: srv.URL, client: &http.Client{Timeout: 5 * time.Second}}
	var deltas []string
	full, err := c.GenerateStream(context.Background(), streamSampleMsgs(), 0.7, func(d string) {
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
		_, _ = w.Write([]byte(`{"type":"error","error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	c := &Client{apiKey: "bad", model: "claude-3-opus", baseURL: srv.URL, client: &http.Client{Timeout: 5 * time.Second}}
	_, err := c.GenerateStream(context.Background(), streamSampleMsgs(), 0.7, nil)
	if err == nil {
		t.Error("expected error for non-200 stream response")
	}
}

func streamSampleMsgs() []vscodeprompt.VSCodeMessage {
	return []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleSystem, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "sys"}}},
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "hi"}}},
	}
}
