package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

// captureStdout redirects os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestGenerateCommitMessage_ExtractAndConventional(t *testing.T) {
	got, err := generateCommitMessage(context.Background(), fakeProvider{resp: "```text\nfeat: x\n```"}, baseMsgs(), 0.7, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "feat: x" {
		t.Errorf("got %q", got)
	}
}

func TestGenerateCommitMessage_Error(t *testing.T) {
	if _, err := generateCommitMessage(context.Background(), fakeProvider{err: errors.New("boom")}, baseMsgs(), 0.7, false); err == nil {
		t.Error("expected error")
	}
}

func TestRunStyle_PrintsLearnedGuidance(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat(cli): add config command PROJ-123")
	writeFile(t, dir, "main.go", "package main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "fix(cli): repair output PROJ-124")

	out := captureStdout(t, func() {
		if err := runStyle(context.Background(), dir, 5); err != nil {
			t.Errorf("runStyle error: %v", err)
		}
	})
	for _, want := range []string{"Repository commit style", "Conventional Commits", "Common scopes", "Ticket references", "do not invent IDs"} {
		if !strings.Contains(out, want) {
			t.Errorf("style output missing %q:\n%s", want, out)
		}
	}
}

func TestRunSuggestNonInteractive_DryRunPrintsNoSideEffect(t *testing.T) {
	hookFile := filepath.Join(t.TempDir(), "MSG")
	cfg := Config{DryRun: true, HookFile: hookFile, Temperature: 0.7, Timeout: time.Second}
	tr := i18n.New(i18n.LocaleEN)
	out := captureStdout(t, func() {
		_ = runSuggestNonInteractive(context.Background(), cfg, "/repo", fakeProvider{resp: "feat: dry"}, baseMsgs(), nil, tr)
	})
	if !strings.Contains(out, "feat: dry") {
		t.Errorf("expected message printed, got %q", out)
	}
	if _, err := os.Stat(hookFile); !os.IsNotExist(err) {
		t.Error("dry-run must not write hook file")
	}
}

func TestRunSuggestNonInteractive_PrintWritesHook(t *testing.T) {
	hookFile := filepath.Join(t.TempDir(), "MSG")
	cfg := Config{Print: true, HookFile: hookFile, Temperature: 0.7, Timeout: time.Second}
	tr := i18n.New(i18n.LocaleEN)
	_ = captureStdout(t, func() {
		if err := runSuggestNonInteractive(context.Background(), cfg, "/repo", fakeProvider{resp: "feat: printed"}, baseMsgs(), nil, tr); err != nil {
			t.Errorf("error: %v", err)
		}
	})
	b, err := os.ReadFile(hookFile)
	if err != nil {
		t.Fatalf("hook file not written: %v", err)
	}
	if strings.TrimSpace(string(b)) != "feat: printed" {
		t.Errorf("hook content = %q", string(b))
	}
}

func TestRunSuggestNonInteractive_GenerateError(t *testing.T) {
	cfg := Config{Print: true, Timeout: time.Second}
	tr := i18n.New(i18n.LocaleEN)
	err := runSuggestNonInteractive(context.Background(), cfg, "/repo", fakeProvider{err: errors.New("down")}, baseMsgs(), nil, tr)
	if err == nil {
		t.Error("expected generation error")
	}
}

func TestProviderLabel(t *testing.T) {
	if providerLabel("") != "openai" {
		t.Error("empty should map to openai")
	}
	if providerLabel("Anthropic") != "anthropic" {
		t.Error("should lowercase")
	}
}

func TestRunModels_UnsupportedProvider(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	for _, p := range []string{"anthropic", "gemini"} {
		err := runModels(context.Background(), Config{Provider: p, Model: "x"}, tr)
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Errorf("%s: expected unsupported error, got %v", p, err)
		}
	}
}

func TestRunModels_Ollama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3"},{"name":"mistral"}]}`))
	}))
	defer srv.Close()

	tr := i18n.New(i18n.LocaleEN)
	out := captureStdout(t, func() {
		if err := runModels(context.Background(), Config{Provider: "ollama", BaseURL: srv.URL}, tr); err != nil {
			t.Errorf("error: %v", err)
		}
	})
	if !strings.Contains(out, "llama3") || !strings.Contains(out, "mistral") {
		t.Errorf("expected model names, got %q", out)
	}
}

func TestRunModels_OllamaCloud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"deepseek-v4-pro"}]}`))
	}))
	defer srv.Close()

	tr := i18n.New(i18n.LocaleEN)
	out := captureStdout(t, func() {
		if err := runModels(context.Background(), Config{Provider: "ollama-cloud", BaseURL: srv.URL, APIKey: "k"}, tr); err != nil {
			t.Errorf("error: %v", err)
		}
	})
	if !strings.Contains(out, "deepseek-v4-pro") {
		t.Errorf("got %q", out)
	}
}

func TestRunModels_OpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("missing auth header")
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-3.5"}]}`))
	}))
	defer srv.Close()

	tr := i18n.New(i18n.LocaleEN)
	out := captureStdout(t, func() {
		if err := runModels(context.Background(), Config{Provider: "openai", BaseURL: srv.URL, APIKey: "k"}, tr); err != nil {
			t.Errorf("error: %v", err)
		}
	})
	if !strings.Contains(out, "gpt-4o") {
		t.Errorf("expected models, got %q", out)
	}
}

func TestRunModels_OpenRouterAndCompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-4.1-mini"}]}`))
	}))
	defer srv.Close()

	tr := i18n.New(i18n.LocaleEN)
	for _, p := range []string{"openrouter", "compatible"} {
		out := captureStdout(t, func() {
			if err := runModels(context.Background(), Config{Provider: p, BaseURL: srv.URL, APIKey: "k"}, tr); err != nil {
				t.Errorf("%s error: %v", p, err)
			}
		})
		if !strings.Contains(out, "openai/gpt-4.1-mini") {
			t.Errorf("%s: got %q", p, out)
		}
	}
}

func TestRunPing_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer srv.Close()

	tr := i18n.New(i18n.LocaleEN)
	out := captureStdout(t, func() {
		if err := runPing(context.Background(), Config{Provider: "openai", BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o", Timeout: 5 * time.Second}, tr); err != nil {
			t.Errorf("ping error: %v", err)
		}
	})
	if !strings.Contains(out, "OK") {
		t.Errorf("expected OK, got %q", out)
	}
}

func TestRunPing_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	tr := i18n.New(i18n.LocaleEN)
	err := runPing(context.Background(), Config{Provider: "openai", BaseURL: srv.URL, APIKey: "bad", Model: "gpt-4o", Timeout: 5 * time.Second}, tr)
	if err == nil {
		t.Error("expected ping failure")
	}
}

func TestRunPing_MissingProvider(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	if err := runPing(context.Background(), Config{Provider: "openai", Model: "gpt-4o"}, tr); err == nil {
		t.Error("expected error when no credentials configured")
	}
}

func TestRun_ConfigPathAndShow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	pathOut := captureStdout(t, func() {
		if err := Run(context.Background(), Config{Command: "config", ConfigAction: "path"}); err != nil {
			t.Errorf("config path error: %v", err)
		}
	})
	if !strings.Contains(filepath.ToSlash(pathOut), ".commitgen.json") {
		t.Errorf("config path output = %q", pathOut)
	}

	showOut := captureStdout(t, func() {
		if err := Run(context.Background(), Config{Command: "config", ConfigAction: "show"}); err != nil {
			t.Errorf("config show error: %v", err)
		}
	})
	if !strings.Contains(showOut, "Config file:") {
		t.Errorf("config show output = %q", showOut)
	}
}

func TestMaskSecret(t *testing.T) {
	if maskSecret("") != "" {
		t.Error("empty should stay empty")
	}
	if maskSecret("super-secret-key") != secretMask {
		t.Error("non-empty should be masked")
	}
}

func TestPreserveSecret(t *testing.T) {
	if got := preserveSecret("", "existing"); got != "existing" {
		t.Errorf("empty submitted = %q, want existing", got)
	}
	if got := preserveSecret(secretMask, "existing"); got != "existing" {
		t.Errorf("mask submitted = %q, want existing", got)
	}
	if got := preserveSecret("new-key", "existing"); got != "new-key" {
		t.Errorf("new key = %q", got)
	}
}
