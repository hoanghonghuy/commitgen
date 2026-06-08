package app

import (
	"context"
	"strings"
	"testing"
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
		{"anthropic missing key", Config{Provider: "anthropic", Model: "claude"}, true},
		{"anthropic ok", Config{Provider: "anthropic", Model: "claude", AnthropicKey: "k"}, false},
		{"gemini missing key", Config{Provider: "gemini", Model: "gemini"}, true},
		{"gemini ok", Config{Provider: "gemini", Model: "gemini", GeminiKey: "k"}, false},
		{"openai missing creds", Config{Provider: "openai", Model: "gpt"}, true},
		{"openai ok with key", Config{Provider: "openai", Model: "gpt", APIKey: "k"}, false},
		{"openai ok with baseurl", Config{Provider: "", Model: "gpt", BaseURL: "http://x"}, false},
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

func TestBuildPromptData(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	// stage a real source file and an ignored lock file
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "go.sum", "h1:abc\n")
	runGit(t, dir, "add", ".")

	data, err := buildPromptData(ctx, dir, 5, 10, false, "custom note", nil)
	if err != nil {
		t.Fatalf("buildPromptData error: %v", err)
	}
	if data.CustomInstructions != "custom note" {
		t.Errorf("custom instructions not set: %q", data.CustomInstructions)
	}
	// go.sum is in defaultIgnores → only main.go remains
	if len(data.Changes) != 1 || data.Changes[0].Path != "main.go" {
		t.Fatalf("expected only main.go, got %+v", data.Changes)
	}
	if !strings.Contains(data.Changes[0].Diff, "package main") {
		t.Errorf("diff missing content: %q", data.Changes[0].Diff)
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
	if err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Errorf("expected 'all ignored' error, got: %v", err)
	}
}
