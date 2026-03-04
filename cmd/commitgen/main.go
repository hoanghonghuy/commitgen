package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/app"
	"github.com/hoanghonghuy/commitgen/internal/config"
)

func main() {
	// 1. Define flags
	cmdFlag := flag.String("cmd", "suggest", "Command to run (suggest | dump-prompt | config | install-hook)")
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

	flag.Parse()

	// Support positional commands (e.g., 'commitgen config' instead of 'commitgen -cmd=config')
	cmd := *cmdFlag
	if flag.NArg() > 0 {
		posCmd := flag.Arg(0)
		switch posCmd {
		case "suggest", "dump-prompt", "config", "install-hook":
			cmd = posCmd
		}
	}

	// 2. Load config from file
	fileCfg, err := config.Load(*configPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
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
	}

	// 4. Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// 5. Run application
	if err := app.Run(ctx, cfg); err != nil {
		if ctx.Err() == context.Canceled {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
