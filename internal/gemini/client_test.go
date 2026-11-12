package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestBuildRequest_ZeroTemperature(t *testing.T) {
	c := New(Config{Model: "test-model"})
	payload, err := json.Marshal(c.buildRequest(sampleMsgs(), 0))
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var request struct {
		GenerationConfig map[string]json.RawMessage `json:"generationConfig"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if got := string(request.GenerationConfig["temperature"]); got != "0" {
		t.Errorf("temperature = %q, want 0", got)
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

func TestGenerate_PreservesAllTextParts(t *testing.T) {
	for _, stream := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := `{"candidates":[{"content":{"parts":[{"thought":true,"text":"PRIVATE_THOUGHT"},{"text":"feat: complete"},{"text":"\n\nBody and footer"}]},"finishReason":"STOP"},{"content":{"parts":[{"text":"OTHER_CANDIDATE"}]}}]}`
			if stream {
				body = "data: " + body + "\n\n"
			}
			_, _ = w.Write([]byte(body))
		}))
		c := newTestClient(srv.URL)
		var got string
		var err error
		var deltas strings.Builder
		if stream {
			got, err = c.GenerateStream(context.Background(), sampleMsgs(), 0.7, func(delta string) { deltas.WriteString(delta) })
		} else {
			got, err = c.Generate(context.Background(), sampleMsgs(), 0.7)
		}
		srv.Close()
		if err != nil || got != "feat: complete\n\nBody and footer" || (stream && deltas.String() != got) {
			t.Errorf("stream=%v: parts lost: content=%q deltas=%q err=%v", stream, got, deltas.String(), err)
		}
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
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"add stream\"}]},\"finishReason\":\"STOP\"}]}\n\n"))
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

func TestGenerateStream_Completion(t *testing.T) {
	for _, tt := range []struct{ name, terminal, wantError string }{
		{"early EOF", "", "unexpected EOF"},
		{"provider error", `{"error":{"code":429,"message":"synthetic overload","status":"RESOURCE_EXHAUSTED"}}`, "synthetic overload"},
		{"token limit", `{"candidates":[{"finishReason":"MAX_TOKENS"}]}`, "MAX_TOKENS"},
		{"finished without text in final chunk", `{"candidates":[{"finishReason":"STOP"}]}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"feat: partial\"}]}}]}\n\n")
				if tt.terminal != "" {
					fmt.Fprintf(w, "data: %s\n\n", tt.terminal)
				}
			}))
			defer srv.Close()
			var deltas strings.Builder
			got, err := newTestClient(srv.URL).GenerateStream(context.Background(), sampleMsgs(), 0.7, func(s string) { deltas.WriteString(s) })
			if tt.wantError == "" {
				if err != nil || got != "feat: partial" {
					t.Fatalf("completed response=%q err=%v", got, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) || got != "" {
				t.Fatalf("incomplete stream accepted: content=%q err=%v", got, err)
			}
			if tt.terminal == "" && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("want unexpected EOF, got %v", err)
			}
			if deltas.String() != "feat: partial" {
				t.Errorf("partial display lost: %q", deltas.String())
			}
		})
	}
}
