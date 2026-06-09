package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func sampleMsgs() []vscodeprompt.VSCodeMessage {
	return []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleSystem, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "sys instr"}}},
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "hi"}}},
		{Role: vscodeprompt.RoleAssistant, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "prev"}}},
	}
}

func newTestClient(baseURL string) *Client {
	return &Client{
		apiKey:  "test-key",
		model:   "gemini-1.5-pro",
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func TestNew_SetsDefaultURL(t *testing.T) {
	c := New(Config{APIKey: "k", Model: "m"})
	if c.baseURL != defaultGeminiBaseURL {
		t.Errorf("expected default URL, got %q", c.baseURL)
	}
}

func TestGenerate_SystemInstructionAndRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("api key not in header: %q", got)
		}
		var req generateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.SystemInstruction == nil || len(req.SystemInstruction.Parts) == 0 ||
			req.SystemInstruction.Parts[0].Text != "sys instr" {
			t.Errorf("system instruction not set: %+v", req.SystemInstruction)
		}
		if req.GenerationConfig == nil || req.GenerationConfig.Temperature != 0.9 {
			t.Errorf("generation config wrong: %+v", req.GenerationConfig)
		}
		// user then assistant→model
		if len(req.Contents) != 2 || req.Contents[0].Role != "user" || req.Contents[1].Role != "model" {
			t.Errorf("role mapping wrong: %+v", req.Contents)
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"docs: update"}]}}]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "docs: update" {
		t.Errorf("got %q", got)
	}
}

func TestGenerate_EmptyCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.9)
	if err == nil {
		t.Error("expected error for empty candidates")
	}
}

func TestGenerate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"denied"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.9)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestGenerateStream_CollectsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("api key not in header: %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"feat: \"}]}}]}\n\n" +
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"add stream\"}]}}]}\n\n"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
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
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"denied"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.GenerateStream(context.Background(), sampleMsgs(), 0.7, nil)
	if err == nil {
		t.Error("expected error for non-200 stream response")
	}
}
