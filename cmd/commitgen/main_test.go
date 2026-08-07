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
		{"unrecognized positional ignored", "suggest", []string{"frobnicate"}, "suggest"},
		{"install-hook positional", "suggest", []string{"install-hook"}, "install-hook"},
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
