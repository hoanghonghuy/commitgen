package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stagedRepo creates a repo with one initial commit and a staged source file.
func stagedRepo(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# init\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	runGit(t, dir, "add", ".")
	return dir
}

func TestRun_DumpPromptToFile(t *testing.T) {
	dir := stagedRepo(t)
	out := filepath.Join(t.TempDir(), "prompt.json")

	cfg := Config{
		Command:     "dump-prompt",
		RepoArg:     dir,
		DumpOutPath: out,
		RecentN:     3,
		MaxFiles:    10,
		Timeout:     time.Second,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run(dump-prompt) error: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("dump output not written: %v", err)
	}
}

func TestRun_InstallAndUninstallHook(t *testing.T) {
	dir := initRepo(t)

	if err := Run(context.Background(), Config{Command: "install-hook", RepoArg: dir}); err != nil {
		t.Fatalf("Run(install-hook) error: %v", err)
	}
	hookPath := filepath.Join(dir, ".git", "hooks", "prepare-commit-msg")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook not installed: %v", err)
	}

	if err := Run(context.Background(), Config{Command: "uninstall-hook", RepoArg: dir}); err != nil {
		t.Fatalf("Run(uninstall-hook) error: %v", err)
	}
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Error("hook should be removed")
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	dir := stagedRepo(t)
	cfg := Config{Command: "bogus", RepoArg: dir, RecentN: 1, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestRun_BadRepoArg(t *testing.T) {
	cfg := Config{Command: "dump-prompt", RepoArg: filepath.Join(t.TempDir(), "missing"), Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error for non-existent repo")
	}
}

func TestRun_MissingInstructionsFile(t *testing.T) {
	dir := stagedRepo(t)
	cfg := Config{
		Command:          "dump-prompt",
		RepoArg:          dir,
		InstructionsPath: filepath.Join(dir, "no-such-instructions.txt"),
		Timeout:          time.Second,
	}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error for missing instructions file")
	}
}

func TestRun_NoStagedChanges(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# init\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")
	// nothing staged now
	cfg := Config{Command: "dump-prompt", RepoArg: dir, RecentN: 1, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error when no staged changes")
	}
}

func TestRun_DumpPromptWithInstructions(t *testing.T) {
	dir := stagedRepo(t)
	instr := filepath.Join(t.TempDir(), "instr.txt")
	if err := os.WriteFile(instr, []byte("always use imperative mood"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "p.json")
	cfg := Config{
		Command:          "dump-prompt",
		RepoArg:          dir,
		InstructionsPath: instr,
		DumpOutPath:      out,
		RecentN:          2,
		MaxFiles:         5,
		Timeout:          time.Second,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	if len(b) == 0 {
		t.Error("expected non-empty dump output")
	}
}
