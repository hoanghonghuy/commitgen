package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/app"
	"github.com/hoanghonghuy/commitgen/internal/config"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/ollama"
)

// Build information, injected via -ldflags at release time (see .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// 1. Define flags
	cmdFlag := flag.String("cmd", "suggest", "Command to run (suggest | review | dump-prompt | config | install-hook | uninstall-hook)")
	repoFlag := flag.String("repo", "", "Path to git repository (default: current directory)")
	baseURLFlag := flag.String("base-url", "", "AI provider base URL")
	apiKeyFlag := flag.String("api-key", "", "AI provider API key")
	modelFlag := flag.String("model", "", "AI model name")
	providerFlag := flag.String("provider", "", "AI provider (openai | ollama | anthropic | gemini)")

	anthropicKeyFlag := flag.String("anthropic-key", "", "Anthropic API key")
	geminiKeyFlag := flag.String("gemini-key", "", "Gemini API key")

	recentNFlag := flag.Int("recent-n", 0, "Number of recent commits to include")
	maxFilesFlag := flag.Int("max-files", 0, "Max staged files to analyze")
	summarizeFlag := flag.Bool("summarize", false, "Summarize file content")
	tempFlag := flag.Float64("temp", 0, "LLM temperature")
	conventionalFlag := flag.Bool("conventional", false, "Enforce conventional commits")

	hookFlag := flag.String("hook", "", "Path to commit message file (used by git hook)")
	dumpOutFlag := flag.String("dump-out", "", "Output path for dump-prompt")
	instructionsFlag := flag.String("instructions", "", "Path to custom instructions file")
	configPathFlag := flag.String("config", "", "Path to config file")

	logLevelFlag := flag.String("log-level", "", "Log level (debug, info, warn, error)")
	logOutputFlag := flag.String("log-output", "", "Log output (stdout, stderr, file, both)")
	logFileFlag := flag.String("log-file", "", "Log file path")

	timeoutFlag := flag.Int("timeout", 0, "AI request timeout in seconds (default 120)")
	printFlag := flag.Bool("print", false, "Print the generated message to stdout without launching the TUI")
	dryRunFlag := flag.Bool("dry-run", false, "Generate and preview without committing")
	amendFlag := flag.Bool("amend", false, "Amend the last commit instead of creating a new one")
	countFlag := flag.Int("count", 1, "Number of commit message candidates to generate")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	localeFlag := flag.String("locale", "", "UI language (en, vi, ja, zh). Default: en")

	flag.Parse()

	// Support positional commands (e.g., 'commitgen config' instead of 'commitgen -cmd=config')
	cmd := resolveCommand(*cmdFlag, flag.Args())

	if *versionFlag || cmd == "version" {
		printVersion()
		return
	}

	// `config show` / `config path` sub-actions
	configAction := ""
	if cmd == "config" && len(flag.Args()) > 1 {
		configAction = flag.Args()[1]
	}

	// 2. Load config from file (global + optional repo-local overlay)
	fileCfg, err := config.LoadResolved(*configPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error loading config: %v\n", err)
	}

	// Resolve locale early so we can use it for warning/error messages.
	locale := config.ResolveString(*localeFlag, os.Getenv("COMMITGEN_LOCALE"), fileCfg.Locale, "en")
	tr := i18n.New(i18n.Locale(locale))

	// 3. Resolve final config (Flag > Env > File > Default)
	timeoutSec := config.ResolveInt(*timeoutFlag, isFlagSet("timeout"), fileCfg.Timeout, 120)
	if timeoutSec <= 0 {
		timeoutSec = 120
	}

	// Prompt template: a file (prompt_template_file) takes precedence over the inline template.
	promptTemplate := fileCfg.PromptTemplate
	if strings.TrimSpace(fileCfg.PromptTemplateFile) != "" {
		if b, readErr := os.ReadFile(fileCfg.PromptTemplateFile); readErr == nil {
			promptTemplate = string(b)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", tr.T("warn.prompt_template_file", fileCfg.PromptTemplateFile, readErr))
		}
	}

	// Resolve the provider first so the default model can be provider-specific
	// (e.g. an Ollama user should not fall back to an OpenAI model name).
	provider := config.ResolveString(*providerFlag, getenvWithFallback("COMMITGEN_PROVIDER", "COMMITAI_PROVIDER", tr), fileCfg.Provider, "openai")

	baseURL := config.ResolveString(*baseURLFlag, getenvWithFallback("COMMITGEN_BASE_URL", "COMMITAI_BASE_URL", tr), fileCfg.BaseURL, "")
	commitgenAPIKey := getenvWithFallback("COMMITGEN_API_KEY", "COMMITAI_API_KEY", tr)
	apiKey := config.ResolveString(*apiKeyFlag, commitgenAPIKey, fileCfg.APIKey, "")
	if strings.EqualFold(provider, "ollama") {
		apiKey = ollama.ResolveAPIKey(*apiKeyFlag, fileCfg.APIKey, commitgenAPIKey)
		baseURL = ollama.ResolveBaseURL(baseURL, apiKey)
	}
	apiKey = strings.TrimSpace(apiKey)

	defaultModel := defaultModelForProvider(provider)
	if strings.EqualFold(provider, "ollama") {
		defaultModel = ollama.DefaultModel(baseURL, apiKey)
	}

	cfg := app.Config{
		Command:  cmd,
		RepoArg:  *repoFlag,
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    config.ResolveString(*modelFlag, getenvWithFallback("COMMITGEN_MODEL", "COMMITAI_MODEL", tr), fileCfg.Model, defaultModel),
		Provider: provider,

		AnthropicKey: config.ResolveString(*anthropicKeyFlag, getenvWithFallback("COMMITGEN_ANTHROPIC_KEY", "COMMITAI_ANTHROPIC_KEY", tr), fileCfg.AnthropicKey, ""),
		GeminiKey:    config.ResolveString(*geminiKeyFlag, getenvWithFallback("COMMITGEN_GEMINI_KEY", "COMMITAI_GEMINI_KEY", tr), fileCfg.GeminiKey, ""),

		RecentN:      config.ResolveInt(*recentNFlag, isFlagSet("recent-n"), fileCfg.RecentN, 5),
		MaxFiles:     config.ResolveInt(*maxFilesFlag, isFlagSet("max-files"), fileCfg.MaxFiles, 10),
		Summarize:    config.ResolveBool(*summarizeFlag, isFlagSet("summarize"), fileCfg.Summarize, true),
		Temperature:  config.ResolveFloat(*tempFlag, isFlagSet("temp"), fileCfg.Temperature, 0.7),
		Conventional: config.ResolveBool(*conventionalFlag, isFlagSet("conventional"), fileCfg.Conventional, true),

		HookFile:         *hookFlag,
		DumpOutPath:      *dumpOutFlag,
		InstructionsPath: *instructionsFlag,
		ConfigPath:       *configPathFlag,
		Timeout:          time.Duration(timeoutSec) * time.Second,
		PromptTemplate:   promptTemplate,
		ReviewLanguage:   config.ResolveString("", "", fileCfg.ReviewLanguage, "en"),
		Locale:           locale,
		RulesFile:        fileCfg.RulesFile,
		PromptTemplateFile: fileCfg.PromptTemplateFile,
		TimeoutSeconds:   fileCfg.Timeout,
		IgnoredFiles:     fileCfg.IgnoredFiles,
		Print:            *printFlag,
		DryRun:           *dryRunFlag,
		Amend:            *amendFlag,
		Count:            *countFlag,
		ConfigAction:     configAction,

		LogLevel:  config.ResolveString(*logLevelFlag, getenvWithFallback("COMMITGEN_LOG_LEVEL", "COMMITAI_LOG_LEVEL", tr), fileCfg.LogLevel, "info"),
		LogOutput: config.ResolveString(*logOutputFlag, getenvWithFallback("COMMITGEN_LOG_OUTPUT", "COMMITAI_LOG_OUTPUT", tr), fileCfg.LogOutput, "both"),
		LogFile:   config.ResolveString(*logFileFlag, getenvWithFallback("COMMITGEN_LOG_FILE", "COMMITAI_LOG_FILE", tr), fileCfg.LogFile, ""),
	}

	// 4. Initialize logger
	loggerCfg := logger.Config{
		Level:      cfg.LogLevel,
		Output:     cfg.LogOutput,
		FilePath:   cfg.LogFile,
		JSONFormat: false,
	}
	if err := logger.Init(loggerCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize logger: %v\n", err)
	}
	defer logger.Close()

	// 5. Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// 6. Run application
	if err := app.Run(ctx, cfg); err != nil {
		if ctx.Err() == context.Canceled {
			os.Exit(0)
		}
		// Log error to file/stderr AFTER TUI exits
		logger.Error("application error", "error", err)
		// Also print to stderr so user sees it immediately
		fmt.Fprintf(os.Stderr, "\n%s\n", tr.T("app.error", app.TranslateError(tr, err)))
		if logPath := resolveLogFilePath(cfg.LogFile, cfg.LogOutput); logPath != "" {
			fmt.Fprintf(os.Stderr, "%s\n", tr.T("app.check_logs", logPath))
		}
		os.Exit(1)
	}
}

// resolveCommand returns the effective command: a recognized positional
// argument takes precedence over the -cmd flag's default value.
func resolveCommand(cmdFlag string, args []string) string {
	cmd := cmdFlag
	if len(args) > 0 {
		switch args[0] {
		case "suggest", "review", "dump-prompt", "config", "install-hook", "uninstall-hook",
			"version", "ping", "models":
			cmd = args[0]
		}
	}
	return cmd
}

// printVersion writes build information to stdout.
func printVersion() {
	fmt.Printf("commitgen %s\ncommit: %s\nbuilt:  %s\n", version, commit, date)
}

// resolveLogFilePath returns the path of the log file that errors are written to,
// or "" when logging is not directed to a file (e.g. stderr/stdout only).
func resolveLogFilePath(logFile, logOutput string) string {
	switch strings.ToLower(logOutput) {
	case "file", "both":
		if logFile != "" {
			return logFile
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "commitgen.log"
		}
		return filepath.Join(home, ".commitgen", "commitgen.log")
	default:
		// stdout / stderr / unknown → no file written
		return ""
	}
}

// defaultModelForProvider returns a sensible default model name for each
// supported provider, so users who only set --provider still get a working
// model instead of always falling back to an OpenAI model.
func defaultModelForProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "ollama":
		return "llama3"
	case "anthropic":
		return "claude-3-opus"
	case "gemini":
		return "gemini-1.5-pro"
	default: // openai or empty
		return "gpt-4o"
	}
}

// getenvWithFallback reads the primary environment variable, falling back to a
// deprecated alias for backward compatibility. When only the deprecated alias
// is set, it prints a one-time deprecation warning to stderr.
func getenvWithFallback(primary, deprecated string, tr *i18n.Translator) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if v := os.Getenv(deprecated); v != "" {
		fmt.Fprintf(os.Stderr, "%s\n", tr.T("warn.deprecated_env", deprecated, primary))
		return v
	}
	return ""
}

func isFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
