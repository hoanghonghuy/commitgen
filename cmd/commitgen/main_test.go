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
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/hoanghonghuy/commitgen/internal/config"
)

type formStep struct {
	prompt string
	input  string
	signal syscall.Signal
}

func TestCLI_ConfigAccessibleNumericDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":"openai","recent_n":7,"max_files":6,"temperature":0.9,"timeout_seconds":42}`), 0600); err != nil {
		t.Fatal(err)
	}
	steps := []formStep{
		{prompt: "Enter a number between 1 and 7: ", input: "\n"},
		{prompt: "Model ", input: "\n"}, {prompt: "API Key ", input: "\n"},
		{prompt: "System Prompt Template ", input: "\n"},
	}
	for _, prompt := range []string{"Recent Commits ", "Max Files ", "Temperature ", "Request Timeout (seconds) ", "Summarize Changes [Y/n] ", "Conventional Commits [Y/n] ", "Ignored Files ", "Rules File ", "Enter a number between 1 and 5: ", "Enter a number between 1 and 4: ", "Enter a number between 1 and 4: ", "Enter a number between 1 and 3: ", "Log File Path "} {
		steps = append(steps, formStep{prompt: prompt, input: "\n"})
	}
	runConfigFormMode(t, path, false, steps)
	saved, err := config.Load(path)
	if err != nil || saved.RecentN == nil || *saved.RecentN != 7 || saved.MaxFiles == nil || *saved.MaxFiles != 6 || saved.Temperature == nil || *saved.Temperature != .9 || saved.Timeout == nil || *saved.Timeout != 42 {
		t.Errorf("Enter changed numeric defaults: saved=%+v err=%v", saved, err)
	}
}

func TestCLI_CLocalePrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "vi_VN.UTF-8")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"provider":"openai"}`), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := runCLI(t, "config", "show", "--config", path, "--locale", "auto")
	if err != nil || !strings.Contains(output, "Config file: ") {
		t.Errorf("LC_ALL=C did not select English: err=%v output=%s", err, output)
	}
}

func TestCLI_ConfigEOFDoesNotSave(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%v", existing), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("ACCESSIBLE", "1")
			t.Setenv("TERM", "dumb")
			path := filepath.Join(t.TempDir(), "config.json")
			before := []byte(`{"provider":"openai","model":"kept-model"}`)
			if existing {
				if err := os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, cliErr := runCLI(t, "config", "--config", path)
			if cliErr == nil || !strings.Contains(out, "EOF") {
				t.Fatalf("expected input EOF, got err=%v output=%s", cliErr, out)
			}
			after, err := os.ReadFile(path)
			if existing && (err != nil || !bytes.Equal(before, after)) {
				t.Errorf("EOF changed config without submission: err=%v content=%s", err, after)
			} else if !existing && !os.IsNotExist(err) {
				t.Errorf("EOF created config without submission: err=%v output=%s", err, out)
			}
		})
	}
}

func TestCLI_ConfigSavePreservesTemplateSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo := t.TempDir()
	initCLIRepo(t, repo)
	t.Chdir(repo)
	for path, content := range map[string]string{
		filepath.Join(repo, ".commitgen.json"): `{"provider":"openai","prompt_template_file":"prompt.txt"}`,
		filepath.Join(repo, "prompt.txt"):      "REPO_TEMPLATE",
		filepath.Join(home, "prompt.txt"):      "OTHER_HOME_TEMPLATE",
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runConfigForm(t, "")
	saved, err := config.Load(filepath.Join(home, ".commitgen.json"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(saved.PromptTemplatePath())
	if err != nil || string(content) != "REPO_TEMPLATE" {
		t.Errorf("saving changed template source: path=%q content=%q err=%v", saved.PromptTemplatePath(), content, err)
	}
}

func TestDefaultModels_SupportedProviders(t *testing.T) {
	// Official lifecycle tables checked on 2026-11-12.
	for provider, want := range map[string]string{"anthropic": "claude-sonnet-4-6", "gemini": "gemini-3.8-flash", "openrouter": "anthropic/claude-sonnet-4.6"} {
		if got := defaultModelForProvider(provider); got != want {
			t.Errorf("%s default model=%q, want supported model %q", provider, got, want)
		}
	}
}

type formDriver struct {
	input    io.Writer
	steps    []formStep
	pending  string
	output   strings.Builder
	terminal bool
	pid      atomic.Int64
}

func (d *formDriver) Write(p []byte) (int, error) {
	d.output.Write(p)
	d.pending += string(p)
	if d.terminal {
		if bytes.Contains(p, []byte("\x1b[6n")) {
			if _, err := io.WriteString(d.input, "\x1b[1;1R"); err != nil {
				return 0, err
			}
		}
		d.pending = ansi.Strip(d.pending)
		if index := strings.Index(d.pending, "COMMITGEN_TEST_PID="); index >= 0 && strings.Contains(d.pending[index:], "\n") {
			var pid int64
			if _, err := fmt.Sscanf(d.pending[index:], "COMMITGEN_TEST_PID=%d", &pid); err == nil {
				d.pid.Store(pid)
			}
		}
	}
	for len(d.steps) > 0 {
		step := d.steps[0]
		index := strings.Index(d.pending, step.prompt)
		if index < 0 {
			break
		}
		d.pending = d.pending[index+len(step.prompt):]
		d.steps = d.steps[1:]
		if step.signal != 0 {
			process, err := os.FindProcess(int(d.pid.Load()))
			if err != nil {
				return 0, err
			}
			if err := process.Signal(step.signal); err != nil {
				return 0, err
			}
		}
		if _, err := io.WriteString(d.input, step.input); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func runConfigForm(t *testing.T, path string, terminalSteps ...[]formStep) string {
	t.Helper()
	if len(terminalSteps) > 0 {
		return runConfigFormMode(t, path, true, terminalSteps[0])
	}
	return runConfigFormMode(t, path, false, nil)
}

func runConfigFormMode(t *testing.T, path string, normal bool, steps []formStep) string {
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
	timeout := 15 * time.Second
	if steps != nil {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		{prompt: "Enter a number between 1 and 7: ", input: "\n"}, {prompt: "Model ", input: "\n"}, {prompt: "API Key ", input: "\n"},
		{prompt: "System Prompt Template ", input: "\n"}, {prompt: "Recent Commits ", input: "5\n"}, {prompt: "Max Files ", input: "10\n"},
		{prompt: "Temperature ", input: "0.7\n"}, {prompt: "Request Timeout (seconds) ", input: "120\n"},
		{prompt: "Summarize Changes [Y/n] ", input: "\n"}, {prompt: "Conventional Commits [Y/n] ", input: "\n"},
		{prompt: "Ignored Files ", input: "\n"}, {prompt: "Rules File ", input: "\n"},
		{prompt: "Enter a number between 1 and 5: ", input: "\n"}, {prompt: "Enter a number between 1 and 4: ", input: "\n"},
		{prompt: "Enter a number between 1 and 4: ", input: "\n"}, {prompt: "Enter a number between 1 and 3: ", input: "\n"}, {prompt: "Log File Path ", input: "\n"},
	}}
	if steps != nil {
		cmd.Env = append(cmd.Env, "NO_COLOR=1", "COMMITGEN_TEST_PTY=1", "GORACE=atexit_sleep_ms=0")
		if normal {
			cmd.Env = append(cmd.Env, "ACCESSIBLE=", "TERM=xterm-256color")
		}
		driver.steps = steps
		driver.terminal = true
		cmd.Cancel = func() error {
			if pid := driver.pid.Load(); pid > 0 {
				if process, err := os.FindProcess(int(pid)); err == nil {
					_ = process.Kill()
				}
			}
			return cmd.Process.Kill()
		}
		cmd.WaitDelay = time.Second
	}
	var stderr bytes.Buffer
	cmd.Stdout = driver
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("config form: %v\n%s\n%s", err, driver.output.String(), stderr.String())
	}
	if len(driver.steps) != 0 {
		t.Fatalf("form finished before all prompts: %s", driver.output.String())
	}
	return driver.output.String()
}

func TestCLI_ConfigTerminalNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	steps := []formStep{{prompt: "┃ AI Provider", input: "\r"}}
	for _, field := range []struct{ title, value string }{
		{"Model", ""}, {"API Key", ""}, {"System Prompt Template", ""},
		{"Number of recent commits to include", " 15 "}, {"Max staged files to analyze", " 7 "}, {"LLM temperature (0.0 - 2.0)", "0.2"}, {"AI request timeout in seconds (default 120)", " 45 "},
		{"Summarize Changes", ""}, {"Conventional Commits", ""}, {"Ignored Files", ""}, {"Rules File", ""},
		{"UI Language", ""}, {"Review Language", ""}, {"Log Level", ""}, {"Log Output", ""}, {"Log File Path", ""},
	} {
		input := "\r"
		if field.value != "" {
			input = "\x01\x0b" + field.value + "\r"
		}
		steps = append(steps, formStep{prompt: "┃ " + field.title, input: input})
	}
	runConfigForm(t, path, steps)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		RecentN  int `json:"recent_n"`
		MaxFiles int `json:"max_files"`
		Timeout  int `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RecentN != 15 || saved.MaxFiles != 7 || saved.Timeout != 45 {
		t.Errorf("accepted settings were lost: %+v", saved)
	}
}

func TestCLI_ConfigProviderAndTemperature(t *testing.T) {
	for _, tt := range []struct{ name, initial, selectKeys, model, invalid, limit string }{
		{name: "provider default", selectKeys: "\x1b[B\x1b[B\x1b[B\x1b[B\x1b[B\r", model: "claude-sonnet-4-6"},
		{name: "custom model retained", initial: `{"provider":"openai","model":"custom-model"}`, selectKeys: "\x1b[B\x1b[B\x1b[B\x1b[B\x1b[B\r", model: "custom-model"},
		{name: "NaN rejected in form", selectKeys: "\r", model: "gpt-4.1-mini", invalid: "NaN", limit: "2.0"},
		{name: "Anthropic range", initial: `{"provider":"anthropic","model":"claude-sonnet-4-6"}`, selectKeys: "\r", model: "claude-sonnet-4-6", invalid: "1.5", limit: "1.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tt.initial != "" {
				if err := os.WriteFile(path, []byte(tt.initial), 0600); err != nil {
					t.Fatal(err)
				}
			}
			steps := []formStep{{prompt: "┃ AI Provider", input: tt.selectKeys}}
			for _, field := range []string{"Model", "API Key", "System Prompt Template", "Number of recent commits to include", "Max staged files to analyze"} {
				steps = append(steps, formStep{prompt: "┃ " + field, input: "\r"})
			}
			temperature := "\r"
			if tt.invalid != "" {
				temperature = "\x01\x0b" + tt.invalid + "\r"
			}
			steps = append(steps, formStep{prompt: "┃ LLM temperature", input: temperature})
			if tt.invalid != "" {
				steps = append(steps, formStep{prompt: "must be between 0.0 and " + tt.limit, input: "\x01\x0b0.7\r"})
			}
			for _, field := range []string{"AI request timeout in seconds (default 120)", "Summarize Changes", "Conventional Commits", "Ignored Files", "Rules File", "UI Language", "Review Language", "Log Level", "Log Output", "Log File Path"} {
				steps = append(steps, formStep{prompt: "┃ " + field, input: "\r"})
			}
			runConfigForm(t, path, steps)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var saved struct {
				Model       string  `json:"model"`
				Temperature float64 `json:"temperature"`
			}
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Model != tt.model || saved.Temperature != .7 {
				t.Errorf("wrong saved form values: model=%q temperature=%v", saved.Model, saved.Temperature)
			}
		})
	}
}

func TestCLI_ConfigAccessibleCancel(t *testing.T) {
	for _, password := range []bool{false, true} {
		for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
			t.Run(fmt.Sprintf("password=%v/signal=%v", password, sig), func(t *testing.T) {
				steps := []formStep{}
				prompt := "Enter a number between 1 and 7: "
				if password {
					steps = append(steps, formStep{prompt: prompt, input: "\n"}, formStep{prompt: "Model ", input: "\n"})
					prompt = "API Key "
				}
				steps = append(steps, formStep{prompt: prompt, signal: sig})
				path := filepath.Join(t.TempDir(), "config.json")
				runConfigFormMode(t, path, false, steps)
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("cancel saved config: %v", err)
				}
			})
		}
	}
}

func TestCLI_ConfigTerminalCancel(t *testing.T) {
	for name, steps := range map[string][]formStep{
		"Esc back and cancel": {{prompt: "┃ AI Provider", input: "\r"}, {prompt: "┃ Model", input: "\x1b"}, {prompt: "┃ AI Provider", input: "\x1b"}},
		"Ctrl+C cancel":       {{prompt: "┃ AI Provider", input: "\x03"}},
		"SIGTERM cancel":      {{prompt: "┃ AI Provider", signal: syscall.SIGTERM}},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			output := runConfigForm(t, path, steps)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("cancel wrote config: %v", err)
			}
			if strings.Contains(output, "Error:") {
				t.Errorf("normal cancellation reported as error: %s", output)
			}
		})
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

func TestCLI_PromptTemplateUsesConfigDirectory(t *testing.T) {
	for _, source := range []string{"repo", "global", "explicit"} {
		t.Run(source, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			repo := t.TempDir()
			initCLIRepo(t, repo)
			if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("staged\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("git", "-C", repo, "add", "file.txt").CombinedOutput(); err != nil {
				t.Fatalf("add: %v %s", err, out)
			}
			configDir := repo
			args := []string{"dump-prompt", "--repo", repo}
			switch source {
			case "global":
				configDir = home
			case "explicit":
				configDir = t.TempDir()
				args = append(args, "--config", filepath.Join(configDir, ".commitgen.json"))
			}
			path := filepath.Join(configDir, ".commitgen.json")
			if err := os.WriteFile(path, []byte(`{"prompt_template_file":"prompt.txt"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "prompt.txt"), []byte("SOURCE_TEMPLATE_MARKER"), 0600); err != nil {
				t.Fatal(err)
			}
			if source == "global" {
				if err := os.WriteFile(filepath.Join(repo, ".commitgen.json"), []byte(`{"review_language":"vi"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(t.TempDir())
			out, err := runCLI(t, args...)
			if err != nil || !strings.Contains(out, "SOURCE_TEMPLATE_MARKER") {
				t.Fatalf("relative template lost: err=%v marker=%v", err, strings.Contains(out, "SOURCE_TEMPLATE_MARKER"))
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != `{"prompt_template_file":"prompt.txt"}` {
				t.Fatalf("loading changed raw config: %s err=%v", data, err)
			}
		})
	}
}

func TestCLIProcess(t *testing.T) {
	if os.Getenv("COMMITGEN_TEST_PROCESS") != "1" {
		return
	}
	if os.Getenv("COMMITGEN_TEST_PTY") == "1" {
		size := exec.Command("stty", "rows", "60", "cols", "80")
		size.Stdin = os.Stdin
		if err := size.Run(); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("COMMITGEN_TEST_PID=%d\n", os.Getpid())
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

func buildCLI(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "commitgen")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return exe
}

func runBinary(t *testing.T, exe, dir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = cliTestEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func initCLIRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "Test"}, {"config", "commit.gpgsign", "false"}, {"commit", "--allow-empty", "-m", "chore: initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git setup: %v: %s", err, out)
		}
	}
}

func TestCLI_InstalledHooksPreserveLiteralPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell path characters are not valid Windows filenames")
	}
	exe := buildCLI(t)
	name := " '$AUDIT_SLOT$(touch AUDIT_CANARY)`touch AUDIT_CANARY`"
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(t.TempDir(), "commitgen"+name)
	if err := os.WriteFile(exe, binary, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("AUDIT_SLOT", "EXPANDED")
	dir := filepath.Join(t.TempDir(), "repo"+name)
	initCLIRepo(t, dir)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"fix: literal paths"}}]}`)
	}))
	defer server.Close()
	path := filepath.Join(dir, "config"+name+".json")
	data, err := json.Marshal(map[string]string{"provider": "compatible", "model": "fixture", "compatible_base_url": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"install-hook", "install-msg-hook"} {
		if out, err := runBinary(t, exe, dir, "--config", path, command); err != nil {
			t.Fatalf("install: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "file.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	cmd := exec.Command("git", "-C", dir, "commit")
	cmd.Env = append(cliTestEnv(), "GIT_EDITOR=true")
	out, commitErr := cmd.CombinedOutput()
	if _, err := os.Stat(filepath.Join(dir, "AUDIT_CANARY")); !os.IsNotExist(err) {
		t.Errorf("hook executed a command from a path: %v", err)
	}
	if commitErr != nil || !strings.Contains(string(out), "fix: literal paths") {
		t.Fatalf("hook did not preserve literal paths: %v %s", commitErr, out)
	}
}

func TestCLI_MsgHookCommentCleanup(t *testing.T) {
	exe := buildCLI(t)
	for _, mode := range []string{"default", "strip", "verbatim", "whitespace"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			repo := t.TempDir()
			initCLIRepo(t, repo)
			for key, value := range map[string]string{"core.commentChar": ";", "commit.cleanup": mode} {
				if mode == "default" && key == "commit.cleanup" {
					continue
				}
				if out, err := exec.Command("git", "-C", repo, "config", key, value).CombinedOutput(); err != nil {
					t.Fatalf("git config: %v %s", err, out)
				}
			}
			if err := os.WriteFile(filepath.Join(repo, ".commitgen-rules.json"), []byte(`{"rules":{"prohibited-words":{"enabled":true,"words":["DO_NOT_USE"]}}}`), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := runBinary(t, exe, repo, "install-msg-hook"); err != nil {
				t.Fatalf("install: %v %s", err, out)
			}
			msgPath := filepath.Join(t.TempDir(), "message.txt")
			if err := os.WriteFile(msgPath, []byte("feat: valid subject\n\n; DO_NOT_USE: editor commentary\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Independent validate-msg must continue validating the raw input.
			if out, err := runBinary(t, exe, repo, "validate-msg", "--file", msgPath); err == nil || !strings.Contains(out, "Prohibited word") {
				t.Fatalf("raw validation changed: err=%v output=%s", err, out)
			}
			cleanup := mode
			if cleanup == "default" {
				cleanup = "strip"
			}
			cmd := exec.Command("git", "-C", repo, "commit", "--allow-empty", "--cleanup="+cleanup, "-F", msgPath)
			cmd.Env = cliTestEnv()
			out, err := cmd.CombinedOutput()
			if mode == "verbatim" || mode == "whitespace" {
				if err == nil || !strings.Contains(string(out), "Prohibited word") {
					t.Fatalf("retained comment was not validated: err=%v output=%s", err, out)
				}
			} else {
				if err != nil {
					t.Fatalf("stripped comment blocked valid commit: %v %s", err, out)
				}
				message, err := exec.Command("git", "-C", repo, "log", "-1", "--format=%B").Output()
				if err != nil || strings.TrimSpace(string(message)) != "feat: valid subject" {
					t.Errorf("wrong final message: %q err=%v", message, err)
				}
			}
		})
	}
}

func TestCLI_MsgHookValidationContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hook integration uses a POSIX shell")
	}
	exe := buildCLI(t)
	for _, scenario := range []string{"worktree", "missing-config", "invalid-config", "invalid-overlay"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", t.TempDir())
			dir := t.TempDir()
			initCLIRepo(t, dir)
			target := dir
			args := []string{"install-msg-hook"}
			configPath := filepath.Join(dir, "policy.json")
			switch scenario {
			case "worktree":
				target = filepath.Join(t.TempDir(), "linked")
				if out, err := exec.Command("git", "-C", dir, "worktree", "add", "-b", "feature/linked", target).CombinedOutput(); err != nil {
					t.Fatalf("worktree: %v %s", err, out)
				}
			case "invalid-overlay":
				configPath = filepath.Join(dir, ".commitgen.json")
			default:
				args = append(args, "--config", configPath)
			}
			rules := filepath.Join(target, ".commitgen-rules.json")
			if scenario != "worktree" {
				rules = filepath.Join(dir, "rules.json")
				if err := os.WriteFile(configPath, []byte(`{"rules_file":"rules.json"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(rules, []byte(`{"rules":{"required-patterns":{"enabled":true,"patterns":["^fix:"]}}}`), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := runBinary(t, exe, dir, args...); err != nil {
				t.Fatalf("install: %v %s", err, out)
			}
			if scenario == "missing-config" {
				if err := os.Remove(configPath); err != nil {
					t.Fatal(err)
				}
			} else if scenario != "worktree" {
				if err := os.WriteFile(configPath, []byte(`{"rules_file":`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(target, "file.txt"), []byte("staged\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("git", "-C", target, "add", "file.txt").CombinedOutput(); err != nil {
				t.Fatalf("add: %v %s", err, out)
			}
			before, err := exec.Command("git", "-C", target, "rev-parse", "HEAD").Output()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("git", "-C", target, "commit", "-m", "feat: disallowed")
			cmd.Env = cliTestEnv()
			out, err := cmd.CombinedOutput()
			after, headErr := exec.Command("git", "-C", target, "rev-parse", "HEAD").Output()
			if err == nil || headErr != nil || !bytes.Equal(before, after) {
				t.Fatalf("hook allowed commit with wrong/missing policy: err=%v headErr=%v output=%s", err, headErr, out)
			}
			if scenario == "worktree" && !strings.Contains(string(out), "required pattern") {
				t.Fatalf("worktree hook failed for an unrelated reason: %s", out)
			}
			if scenario != "worktree" {
				if err := os.WriteFile(configPath, []byte(`{"rules_file":"rules.json"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd = exec.Command("git", "-C", target, "commit", "-m", "fix: allowed")
			cmd.Env = cliTestEnv()
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("valid commit rejected: %v %s", err, out)
			}
		})
	}
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
