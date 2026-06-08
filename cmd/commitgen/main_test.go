package main

import (
	"path/filepath"
	"strings"
	"testing"
)

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
