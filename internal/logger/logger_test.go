package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "debug",
		Output:     "file",
		FilePath:   logPath,
		JSONFormat: false,
	}

	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	// Test logging
	Info("test message", "key", "value")

	// Check file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file was not created")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"invalid", "INFO"}, // default
	}

	for _, tt := range tests {
		level := parseLevel(tt.input)
		if level.String() != tt.want {
			t.Errorf("parseLevel(%q) = %v; want %v", tt.input, level, tt.want)
		}
	}
}

func TestLogError(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "error.log")

	cfg := Config{
		Level:    "error",
		Output:   "file",
		FilePath: logPath,
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	testErr := LogError(os.ErrNotExist, "test error")
	if testErr != os.ErrNotExist {
		t.Error("LogError should return the original error")
	}
}
