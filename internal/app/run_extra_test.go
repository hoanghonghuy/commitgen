package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/config"
)

func TestTruncateUTF8(t *testing.T) {
	// ASCII shorter than limit unchanged
	if got := truncateUTF8("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	// ASCII truncated exactly
	if got := truncateUTF8("hello", 3); got != "hel" {
		t.Errorf("got %q", got)
	}
	// Multi-byte must not split a rune. "é" is 2 bytes (0xC3 0xA9).
	s := "aé" // bytes: 'a'(1) + 0xC3 0xA9
	got := truncateUTF8(s, 2)
	if !isValidUTF8(got) {
		t.Errorf("result is not valid UTF-8: %q (% x)", got, got)
	}
	if got != "a" {
		t.Errorf("expected backed-off truncation to 'a', got %q", got)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"missing model", Config{Provider: "openai"}, true},
		{"ollama ok", Config{Provider: "ollama", Model: "llama3"}, false},
		{"ollama cloud missing key", Config{Provider: "ollama", Model: "deepseek-v4-pro", BaseURL: "https://ollama.com"}, true},
		{"ollama cloud ok", Config{Provider: "ollama", Model: "deepseek-v4-pro", BaseURL: "https://ollama.com", APIKey: "sk-test"}, false},
		{"anthropic missing key", Config{Provider: "anthropic", Model: "claude"}, true},
		{"anthropic ok", Config{Provider: "anthropic", Model: "claude", AnthropicKey: "k"}, false},
		{"gemini missing key", Config{Provider: "gemini", Model: "gemini"}, true},
		{"gemini ok", Config{Provider: "gemini", Model: "gemini", GeminiKey: "k"}, false},
		{"openai missing creds", Config{Provider: "openai", Model: "gpt"}, true},
		{"openai ok with key", Config{Provider: "openai", Model: "gpt", APIKey: "k"}, false},
		{"openai ok with baseurl", Config{Provider: "", Model: "gpt", BaseURL: "http://x"}, false},
		{"openrouter ok", Config{Provider: "openrouter", Model: "m", APIKey: "k", BaseURL: "https://openrouter.ai/api/v1"}, false},
		{"compatible ok", Config{Provider: "compatible", Model: "m", APIKey: "k", BaseURL: "http://proxy"}, false},
		{"ollama-cloud missing key", Config{Provider: "ollama-cloud", Model: "deepseek-v4-pro", BaseURL: "https://ollama.com"}, true},
		{"ollama-cloud ok", Config{Provider: "ollama-cloud", Model: "deepseek-v4-pro", BaseURL: "https://ollama.com", APIKey: "sk-test"}, false},
		{"unknown provider", Config{Provider: "weird", Model: "gpt"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := newProvider(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got provider %v", p)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if p == nil {
				t.Error("expected non-nil provider")
			}
		})
	}
}

type providerTimeoutTransport struct {
	deadline    time.Time
	started     time.Time
	destination string
}

func (transport *providerTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport.started = time.Now()
	transport.destination = req.URL.String()
	transport.deadline, _ = req.Context().Deadline()
	body := `{"choices":[{"message":{"content":"ok"}}],"content":[{"text":"ok"}],"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"message":{"content":"ok"}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestNewProvider_RequestTimeout(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	for _, providerID := range []string{"openai", "openrouter", "compatible", "anthropic", "gemini", "ollama", "ollama-cloud"} {
		t.Run(providerID, func(t *testing.T) {
			for _, test := range []struct {
				name    string
				timeout time.Duration
				want    time.Duration
			}{
				{"configured", 3 * time.Minute, 3 * time.Minute},
				{"default", 0, 2 * time.Minute},
				{"negative", -time.Second, 2 * time.Minute},
			} {
				t.Run(test.name, func(t *testing.T) {
					transport := &providerTimeoutTransport{}
					http.DefaultTransport = transport
					provider, err := newProvider(Config{
						Provider:     providerID,
						Model:        "test-model",
						APIKey:       "test-key",
						AnthropicKey: "test-key",
						GeminiKey:    "test-key",
						Timeout:      test.timeout,
					})
					if err != nil {
						t.Fatalf("new provider: %v", err)
					}
					if _, err := provider.Generate(context.Background(), nil, 0); err != nil {
						t.Fatalf("generate: %v", err)
					}
					if transport.deadline.IsZero() {
						t.Fatal("request has no timeout deadline")
					}
					got := transport.deadline.Sub(transport.started)
					if got > test.want || got < test.want-time.Second {
						t.Errorf("request timeout = %s, want approximately %s", got, test.want)
					}
				})
			}
		})
	}
}

func TestNewProvider_LocalOllamaKeepsConfiguredURL(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	t.Setenv("OLLAMA_API_KEY", "")
	for _, host := range []string{"http://127.0.0.1:11434/proxy/ollama.cloud", "http://ollama.cloud.local:11434"} {
		transport := &providerTimeoutTransport{}
		http.DefaultTransport = transport
		credentials := config.ResolveCredentials(config.CredentialInput{File: config.FileConfig{Provider: "ollama", OllamaBaseURL: host, APIKeys: map[string]string{"ollama": "synthetic-key"}}})
		provider, err := newProvider(Config{Provider: credentials.Provider, BaseURL: credentials.BaseURL, APIKey: credentials.APIKey, Model: "fixture"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Generate(context.Background(), baseMsgs(), 0.7); err != nil {
			t.Fatal(err)
		}
		if transport.destination != host+"/api/chat" {
			t.Errorf("configured=%q requested=%q", host, transport.destination)
		}
	}
}

func TestBuildPromptData(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")
	writeFile(t, dir, ".gitmessage", "type(scope): subject\n\nbody\n")

	// stage a real source file and an ignored lock file
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "go.sum", "h1:abc\n")
	runGit(t, dir, "add", "main.go", "go.sum")

	data, err := buildPromptData(ctx, dir, 5, 10, false, "custom note", nil)
	if err != nil {
		t.Fatalf("buildPromptData error: %v", err)
	}
	if data.CustomInstructions != "custom note" {
		t.Errorf("custom instructions not set: %q", data.CustomInstructions)
	}
	if data.CommitTemplate != "type(scope): subject\n\nbody" {
		t.Errorf("commit template not loaded: %q", data.CommitTemplate)
	}
	if !strings.Contains(data.CommitStyleGuidance, "Conventional Commits") {
		t.Errorf("commit style guidance not loaded: %q", data.CommitStyleGuidance)
	}
	// go.sum is in defaultIgnores → only main.go remains
	if len(data.Changes) != 1 || data.Changes[0].Path != "main.go" {
		t.Fatalf("expected only main.go, got %+v", data.Changes)
	}
	if !strings.Contains(data.Changes[0].Diff, "package main") {
		t.Errorf("diff missing content: %q", data.Changes[0].Diff)
	}
}

func TestBuildPromptData_ExcludesUnstagedContent(t *testing.T) {
	for _, initial := range []string{"new", "empty"} {
		t.Run(initial, func(t *testing.T) {
			dir := stagedRepo(t)
			if initial == "empty" {
				writeFile(t, dir, "main.go", "")
				runGit(t, dir, "add", "main.go")
				runGit(t, dir, "commit", "-m", "chore: add empty file")
			}
			writeFile(t, dir, "main.go", "package main\n// staged content\n")
			runGit(t, dir, "add", "main.go")
			writeFile(t, dir, "main.go", "package main\n// UNSTAGED_PRIVATE_CONTENT\n")
			data, err := buildPromptData(context.Background(), dir, 0, 10, false, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(data.Changes) != 1 {
				t.Fatalf("changes = %+v", data.Changes)
			}
			if strings.Contains(data.Changes[0].OriginalCode, "UNSTAGED_PRIVATE_CONTENT") {
				t.Fatal("AI attachment includes unstaged private content")
			}
			if initial == "new" && !strings.Contains(data.Changes[0].OriginalCode, "staged content") {
				t.Fatal("new-file attachment must contain index content")
			}
		})
	}
}

func TestBuildPromptData_NoStagedChanges(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	_, err := buildPromptData(ctx, dir, 5, 10, false, "", nil)
	if err == nil {
		t.Error("expected error when no staged changes")
	}
}

func TestBuildPromptData_AllIgnored(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	writeFile(t, dir, "go.sum", "h1:abc\n")
	runGit(t, dir, "add", ".")

	_, err := buildPromptData(ctx, dir, 5, 10, false, "", nil)
	if err == nil || !errors.Is(err, ErrAllFilesIgnored) {
		t.Errorf("expected ErrAllFilesIgnored, got: %v", err)
	}
}

func TestBuildPromptData_ValidFileAfterIgnoredPrefix(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	// Stage many ignored lockfiles first, then a real source file.
	for i := 0; i < 25; i++ {
		name := fmt.Sprintf("locks/pkg%d.lock", i)
		writeFile(t, dir, name, "lock\n")
	}
	writeFile(t, dir, "main.go", "package main\n")
	runGit(t, dir, "add", ".")

	data, err := buildPromptData(ctx, dir, 5, 10, false, "", []string{"locks/"})
	if err != nil {
		t.Fatalf("expected main.go after skipped locks, got error: %v", err)
	}
	if len(data.Changes) != 1 || data.Changes[0].Path != "main.go" {
		t.Fatalf("expected main.go, got %+v", data.Changes)
	}
}
