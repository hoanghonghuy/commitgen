package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func sampleMsgs() []vscodeprompt.VSCodeMessage {
	return []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleSystem, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "sys"}}},
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "hi"}}},
		{Role: vscodeprompt.RoleAssistant, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "prev"}}},
	}
}

func TestNew_DefaultsAndTrim(t *testing.T) {
	c := New(Config{Model: "llama3"})
	if c.baseURL != "http://localhost:11434" {
		t.Errorf("expected default base URL, got %q", c.baseURL)
	}
	c2 := New(Config{BaseURL: "http://host:1234/", Model: "llama3"})
	if c2.baseURL != "http://host:1234" {
		t.Errorf("trailing slash not trimmed: %q", c2.baseURL)
	}
}

func TestGenerate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/chat") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			t.Error("stream should be false")
		}
		if req.Options.Temperature != 0.5 {
			t.Errorf("temperature not propagated: %v", req.Options.Temperature)
		}
		// Verify roles mapped correctly
		roles := make([]string, len(req.Messages))
		for i, m := range req.Messages {
			roles[i] = m.Role
		}
		if strings.Join(roles, ",") != "system,user,assistant" {
			t.Errorf("role mapping wrong: %v", roles)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"chore: tidy"},"done":true}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "llama3"})
	got, err := c.Generate(context.Background(), sampleMsgs(), 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "chore: tidy" {
		t.Errorf("got %q", got)
	}
}

func TestGenerate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "llama3"})
	_, err := c.Generate(context.Background(), sampleMsgs(), 0.5)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}
