package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"os/signal"
	"syscall"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/app"
	"github.com/hoanghonghuy/commitgen/internal/config"
	"github.com/hoanghonghuy/commitgen/internal/logger"
)

func main() {
	// 1. Define flags
	cmdFlag := flag.String("cmd", "suggest", "Command to run (suggest | dump-prompt | config | install-hook | uninstall-hook)")
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

	flag.Parse()

	// Support positional commands (e.g., 'commitgen config' instead of 'commitgen -cmd=config')
	cmd := *cmdFlag
	if flag.NArg() > 0 {
		posCmd := flag.Arg(0)
		switch posCmd {
		case "suggest", "dump-prompt", "config", "install-hook", "uninstall-hook":
			cmd = posCmd
		}
	}

	// 2. Load config from file
	fileCfg, err := config.Load(*configPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error loading config: %v\n", err)
	}

	// 3. Resolve final config (Flag > Env > File > Default)
	cfg := app.Config{
		Command:      cmd,
		RepoArg:      *repoFlag,
		BaseURL:      config.ResolveString(*baseURLFlag, os.Getenv("COMMITAI_BASE_URL"), fileCfg.BaseURL, ""),
		APIKey:       config.ResolveString(*apiKeyFlag, os.Getenv("COMMITAI_API_KEY"), fileCfg.APIKey, ""),
		Model:        config.ResolveString(*modelFlag, os.Getenv("COMMITAI_MODEL"), fileCfg.Model, "gpt-4o"),
		Provider:     config.ResolveString(*providerFlag, os.Getenv("COMMITAI_PROVIDER"), fileCfg.Provider, "openai"),
		
		AnthropicKey: config.ResolveString(*anthropicKeyFlag, os.Getenv("COMMITAI_ANTHROPIC_KEY"), fileCfg.AnthropicKey, ""),
		GeminiKey:    config.ResolveString(*geminiKeyFlag, os.Getenv("COMMITAI_GEMINI_KEY"), fileCfg.GeminiKey, ""),

		RecentN:      config.ResolveInt(*recentNFlag, isFlagSet("recent-n"), fileCfg.RecentN, 5),
		MaxFiles:     config.ResolveInt(*maxFilesFlag, isFlagSet("max-files"), fileCfg.MaxFiles, 10),
		Summarize:    config.ResolveBool(*summarizeFlag, isFlagSet("summarize"), fileCfg.Summarize, true),
		Temperature:  config.ResolveFloat(*tempFlag, isFlagSet("temp"), fileCfg.Temperature, 0.7),
		Conventional: config.ResolveBool(*conventionalFlag, isFlagSet("conventional"), fileCfg.Conventional, true),
		
		HookFile:         *hookFlag,
		DumpOutPath:      *dumpOutFlag,
		InstructionsPath: *instructionsFlag,
		ConfigPath:       *configPathFlag,
		Timeout:          60 * time.Second,
		PromptTemplate:   fileCfg.PromptTemplate,
		
		LogLevel:  config.ResolveString(*logLevelFlag, os.Getenv("COMMITAI_LOG_LEVEL"), fileCfg.LogLevel, "info"),
		LogOutput: config.ResolveString(*logOutputFlag, os.Getenv("COMMITAI_LOG_OUTPUT"), fileCfg.LogOutput, "both"),
		LogFile:   config.ResolveString(*logFileFlag, os.Getenv("COMMITAI_LOG_FILE"), fileCfg.LogFile, ""),
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
	
	logger.Info("commitgen started", "command", cfg.Command, "version", "dev")

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
			logger.Info("operation cancelled by user")
			os.Exit(0)
		}
		// Log error to file/stderr AFTER TUI exits
		logger.Error("application error", "error", err)
		// Also print to stderr so user sees it immediately
		fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Check logs at: %s\n", getLogPath(cfg.LogFile))
		os.Exit(1)
	}
	logger.Info("commitgen completed successfully")
}

func getLogPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "commitgen.log"
	}
	return filepath.Join(home, ".commitgen", "commitgen.log")
}
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
