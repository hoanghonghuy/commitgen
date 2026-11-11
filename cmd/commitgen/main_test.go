package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type formStep struct {
	prompt string
	input  string
}

type formDriver struct {
	input   io.Writer
	steps   []formStep
	pending string
	output  strings.Builder
}

func (d *formDriver) Write(p []byte) (int, error) {
	d.output.Write(p)
	d.pending += string(p)
	for len(d.steps) > 0 {
		step := d.steps[0]
		index := strings.Index(d.pending, step.prompt)
		if index < 0 {
			break
		}
		d.pending = d.pending[index+len(step.prompt):]
		d.steps = d.steps[1:]
		if _, err := io.WriteString(d.input, step.input); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func runConfigForm(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("interactive CLI test uses Linux script PTY")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script PTY utility is unavailable")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := quote(exe) + " -test.run=^TestCLIProcess$ -- --config " + quote(path) + " config"
	cmd := exec.CommandContext(ctx, script, "-q", "-e", "-c", command, "/dev/null")
	cmd.Env = append(cliTestEnv(), "ACCESSIBLE=1", "TERM=dumb", "COMMITGEN_LOCALE=en")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	driver := &formDriver{input: input, steps: []formStep{
		{"Enter a number between 1 and 7: ", "\n"}, {"Model ", "\n"}, {"API Key ", "\n"},
		{"System Prompt Template ", "\n"}, {"Recent Commits ", "5\n"}, {"Max Files ", "10\n"},
		{"Temperature ", "0.7\n"}, {"Request Timeout (seconds) ", "120\n"},
		{"Summarize Changes [Y/n] ", "\n"}, {"Conventional Commits [Y/n] ", "\n"},
		{"Ignored Files ", "\n"}, {"Rules File ", "\n"},
		{"Enter a number between 1 and 5: ", "\n"}, {"Enter a number between 1 and 4: ", "\n"},
		{"Enter a number between 1 and 4: ", "\n"}, {"Enter a number between 1 and 3: ", "\n"}, {"Log File Path ", "\n"},
	}}
	var stderr bytes.Buffer
	cmd.Stdout = driver
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("config form: %v\n%s\n%s", err, driver.output.String(), stderr.String())
	}
	if len(driver.steps) != 0 {
		t.Fatalf("form finished before all prompts: %s", driver.output.String())
	}
}

func TestCLI_ConfigKeepsLocalOllamaWithSavedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":"ollama","model":"llama3","api_keys":{"ollama":"synthetic-key"},"log_output":"stderr"}`), 0600); err != nil {
		t.Fatal(err)
	}
	runConfigForm(t, path)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ Provider string }
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Provider != "ollama" {
		t.Fatalf("unchanged Local provider became %q", saved.Provider)
	}
}

func TestCLI_ConfigPreservesAutoLocale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":"ollama","model":"llama3","locale":"auto","log_output":"stderr"}`), 0600); err != nil {
		t.Fatal(err)
	}
	runConfigForm(t, path)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct{ Locale string }
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Locale != "auto" {
		t.Fatalf("unchanged auto setting became %q", saved.Locale)
	}
}

func TestCLI_RepoConfigFollowsTargetRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	current := t.TempDir()
	target := t.TempDir()
	for dir, prompt := range map[string]string{current: "CURRENT_REPO_PROMPT", target: "TARGET_REPO_PROMPT"} {
		for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "chore: initial"}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git setup: %v: %s", err, out)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, ".commitgen.json"), []byte(fmt.Sprintf(`{"prompt_template":%q}`, prompt)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "file.txt"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", target, "add", "file.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	subdir := filepath.Join(target, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(current)
	out, err := runCLI(t, "dump-prompt", "--repo", subdir)
	if err != nil || !strings.Contains(out, "TARGET_REPO_PROMPT") || strings.Contains(out, "CURRENT_REPO_PROMPT") {
		t.Fatalf("wrong repo configuration: err=%v output=%s", err, out)
	}
}

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
	cmd.Env = cliTestEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func cliTestEnv() []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "COMMITGEN_") && !strings.HasPrefix(value, "COMMITAI_") && !strings.HasPrefix(value, "OLLAMA_API_KEY=") {
			env = append(env, value)
		}
	}
	return append(env, "COMMITGEN_TEST_PROCESS=1", "COMMITGEN_LOG_OUTPUT=stderr")
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
