package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIProcess(t *testing.T) {
	if os.Getenv("COMMITGEN_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"commitgen"}, os.Args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet("commitgen", flag.ExitOnError)
	main()
	os.Exit(0)
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "COMMITGEN_") && !strings.HasPrefix(value, "COMMITAI_") && !strings.HasPrefix(value, "OLLAMA_API_KEY=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "COMMITGEN_TEST_PROCESS=1", "COMMITGEN_LOG_OUTPUT=stderr")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestCLI_SubcommandConfigFlags(t *testing.T) {
	before := filepath.Join(t.TempDir(), "before.json")
	after := filepath.Join(t.TempDir(), "after.json")
	for _, args := range [][]string{
		{"--config", before, "config", "path", "--config", after},
		{"--config", before, "config", "--config=" + after, "path"},
		{"--config", before, "--cmd=config", "path", "--config", after},
		{"--config", before, "config", "PATH", "--config", after},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := runCLI(t, args...)
			if err != nil || strings.TrimSpace(out) != after {
				t.Fatalf("config path = %q, err=%v; want %q", out, err, after)
			}
		})
	}
	out, err := runCLI(t, "--config", before, "config", "path", "--unknown-option")
	if err == nil || !strings.Contains(out, "flag provided but not defined") {
		t.Fatalf("unknown flag was not rejected: output=%q err=%v", out, err)
	}
}

func TestCLI_SuggestFlagsAfterCommand(t *testing.T) {
	type generationRequest struct {
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature"`
	}
	requests := make(chan generationRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request generationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- request
		fmt.Fprint(w, `{"choices":[{"message":{"content":"feat: generated"}}]}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "tester@example.com"}, {"config", "user.name", "Tester"}, {"-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "chore: init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git setup: %v: %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "file.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	configPath := filepath.Join(t.TempDir(), "empty.json")
	fileConfig := fmt.Sprintf(`{"provider":"compatible","compatible_base_url":%q,"model":"initial-model"}`, server.URL)
	if err := os.WriteFile(configPath, []byte(fileConfig), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "--config", configPath, "suggest", "--repo", dir, "--provider", "compatible", "--base-url", server.URL, "--model=regression-model", "--temp=0", "--print", "--dry-run", "--count", "2", "--timeout", "5")
	if err != nil || !strings.Contains(out, "1. feat: generated") || !strings.Contains(out, "2. feat: generated") {
		t.Fatalf("suggest output=%q err=%v", out, err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	for len(requests) > 0 {
		req := <-requests
		if req.Model != "regression-model" || req.Temperature != 0 {
			t.Fatalf("flags not applied to request: %v", req)
		}
	}
}

func TestResolveLogFilePath(t *testing.T) {
	// explicit file path with file/both output is returned as-is
	if got := resolveLogFilePath("/tmp/my.log", "file"); got != "/tmp/my.log" {
		t.Errorf("file output = %q", got)
	}
	if got := resolveLogFilePath("/tmp/my.log", "both"); got != "/tmp/my.log" {
		t.Errorf("both output = %q", got)
	}

	// stderr/stdout output → no file path
	if got := resolveLogFilePath("/tmp/my.log", "stderr"); got != "" {
		t.Errorf("stderr output should yield no path, got %q", got)
	}
	if got := resolveLogFilePath("", "stdout"); got != "" {
		t.Errorf("stdout output should yield no path, got %q", got)
	}

	// file output with empty path → default under home
	got := resolveLogFilePath("", "file")
	if got == "" || !strings.Contains(filepath.ToSlash(got), ".commitgen/commitgen.log") {
		t.Errorf("expected default log path, got %q", got)
	}
}

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmdFlag string
		args    []string
		want    string
	}{
		{"default when no args", "suggest", nil, "suggest"},
		{"positional overrides", "suggest", []string{"review"}, "review"},
		{"positional config", "suggest", []string{"config"}, "config"},
		{"positional pr", "suggest", []string{"pr"}, "pr"},
		{"positional style", "suggest", []string{"style"}, "style"},
		{"positional validate-msg", "suggest", []string{"validate-msg"}, "validate-msg"},
		{"unrecognized positional ignored", "suggest", []string{"frobnicate"}, "suggest"},
		{"install-hook positional", "suggest", []string{"install-hook"}, "install-hook"},
		{"install-msg-hook positional", "suggest", []string{"install-msg-hook"}, "install-msg-hook"},
		{"uninstall-msg-hook positional", "suggest", []string{"uninstall-msg-hook"}, "uninstall-msg-hook"},
		{"flag default kept", "dump-prompt", []string{}, "dump-prompt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveCommand(tt.cmdFlag, tt.args); got != tt.want {
				t.Errorf("resolveCommand(%q,%v) = %q; want %q", tt.cmdFlag, tt.args, got, tt.want)
			}
		})
	}
}

func TestIsFlagSet(t *testing.T) {
	// No flags parsed in this test process → arbitrary name is not set.
	if isFlagSet("definitely-not-a-defined-flag") {
		t.Error("expected unset flag to report false")
	}
}
