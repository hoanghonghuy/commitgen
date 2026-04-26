package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	defaultLogger *slog.Logger
	logFile       *os.File
)

type Config struct {
	Level      string // debug, info, warn, error
	Output     string // stdout, stderr, file, both
	FilePath   string // path to log file
	JSONFormat bool   // use JSON format instead of text
}

func Init(cfg Config) error {
	level := parseLevel(cfg.Level)
	
	var writers []io.Writer
	
	// Setup outputs
	switch strings.ToLower(cfg.Output) {
	case "stdout":
		writers = append(writers, os.Stdout)
	case "stderr":
		writers = append(writers, os.Stderr)
	case "file":
		if cfg.FilePath == "" {
			cfg.FilePath = getDefaultLogPath()
		}
		f, err := openLogFile(cfg.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		logFile = f
		writers = append(writers, f)
	case "both":
		writers = append(writers, os.Stderr)
		if cfg.FilePath == "" {
			cfg.FilePath = getDefaultLogPath()
		}
		f, err := openLogFile(cfg.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		logFile = f
		writers = append(writers, f)
	default:
		writers = append(writers, os.Stderr)
	}
	
	writer := io.MultiWriter(writers...)
	
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Redact sensitive keys
			if a.Key == "api_key" || a.Key == "apiKey" || a.Key == "token" || a.Key == "password" {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}
	
	if cfg.JSONFormat {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}
	
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
	
	return nil
}

func Close() {
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getDefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "commitgen.log"
	}
	logDir := filepath.Join(home, ".commitgen")
	os.MkdirAll(logDir, 0755)
	return filepath.Join(logDir, "commitgen.log")
}

func openLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	
	// Open in append mode
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// Convenience functions
func Debug(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Debug(msg, args...)
	}
}

func Info(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Info(msg, args...)
	}
}

func Warn(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Warn(msg, args...)
	}
}

func Error(msg string, args ...any) {
	if defaultLogger != nil {
		defaultLogger.Error(msg, args...)
	}
}

func With(args ...any) *slog.Logger {
	if defaultLogger != nil {
		return defaultLogger.With(args...)
	}
	return slog.Default()
}

// LogError logs error and returns it (for chaining)
func LogError(err error, msg string, args ...any) error {
	if err != nil && defaultLogger != nil {
		allArgs := append([]any{"error", err}, args...)
		defaultLogger.Error(msg, allArgs...)
	}
	return err
}

// Session creates a logger with session ID for tracing
func Session(sessionID string) *slog.Logger {
	return With("session_id", sessionID, "timestamp", time.Now().Format(time.RFC3339))
}
