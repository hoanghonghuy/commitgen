package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDefaultLogPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)

	p := getDefaultLogPath()
	if !strings.Contains(filepath.ToSlash(p), ".commitgen/commitgen.log") {
		t.Errorf("unexpected default log path: %q", p)
	}
}

func TestOpenLogFile_CreatesDirs(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "deep", "nested", "log.txt")
	f, err := openLogFile(nested)
	if err != nil {
		t.Fatalf("openLogFile error: %v", err)
	}
	defer f.Close()
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	cfg := Config{Level: "info", Output: "file", FilePath: filepath.Join(t.TempDir(), "c.log")}
	if err := Init(cfg); err != nil {
		t.Fatal(err)
	}
	// Closing twice must not panic.
	Close()
	Close()
}

func TestInit_FileOutputDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)

	// Empty FilePath with "file" output → falls back to default path under home.
	cfg := Config{Level: "debug", Output: "file"}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	defer Close()
	Info("written to default path")

	def := filepath.Join(dir, ".commitgen", "commitgen.log")
	if _, err := os.Stat(def); err != nil {
		t.Errorf("default log file not created: %v", err)
	}
}
