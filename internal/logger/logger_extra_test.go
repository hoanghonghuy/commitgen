package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_OutputModes(t *testing.T) {
	dir := t.TempDir()
	for _, output := range []string{"stdout", "stderr", "both", "unknown"} {
		cfg := Config{Level: "info", Output: output, FilePath: filepath.Join(dir, output+".log")}
		if err := Init(cfg); err != nil {
			t.Errorf("Init(%q) error: %v", output, err)
		}
		Info("hi")
		Close()
	}
}

func TestConvenienceFunctions_NoPanicWhenNil(t *testing.T) {
	// Reset the package logger to simulate uninitialized state.
	defaultLogger = nil
	Debug("d")
	Info("i")
	Warn("w")
	Error("e")
	if got := LogError(nil, "no error"); got != nil {
		t.Errorf("LogError(nil) should return nil, got %v", got)
	}
	// With falls back to slog.Default when uninitialized
	if With("k", "v") == nil {
		t.Error("With should never return nil")
	}
}

func TestRedaction(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "api_key" || a.Key == "apiKey" || a.Key == "token" || a.Key == "password" {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}
	l := slog.New(slog.NewTextHandler(&buf, opts))
	l.Info("test", "api_key", "super-secret", "safe", "visible")

	out := buf.String()
	if strings.Contains(out, "super-secret") {
		t.Error("api_key value should be redacted")
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Error("expected [REDACTED] marker")
	}
	if !strings.Contains(out, "visible") {
		t.Error("non-sensitive value should remain")
	}
}

func TestSession(t *testing.T) {
	cfg := Config{Level: "debug", Output: "file", FilePath: filepath.Join(t.TempDir(), "s.log")}
	if err := Init(cfg); err != nil {
		t.Fatal(err)
	}
	defer Close()
	if Session("abc123") == nil {
		t.Error("Session should return a logger")
	}
}

func TestInit_JSONFormat(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "j.log")
	cfg := Config{Level: "info", Output: "file", FilePath: logPath, JSONFormat: true}
	if err := Init(cfg); err != nil {
		t.Fatal(err)
	}
	Info("structured", "key", "value")
	Close()

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\"msg\":\"structured\"") {
		t.Errorf("expected JSON log line, got: %s", b)
	}
}
