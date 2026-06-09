package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/anthropic"
	"github.com/hoanghonghuy/commitgen/internal/config"
	"github.com/hoanghonghuy/commitgen/internal/gemini"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/ollama"
	"github.com/hoanghonghuy/commitgen/internal/openai"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"

	tea "github.com/charmbracelet/bubbletea"
)

// runTUI runs a Bubble Tea program for the given model and returns the final
// model. It is a package-level variable so tests can substitute a stub instead
// of launching a real terminal program.
var runTUI = func(model tea.Model) (tea.Model, error) {
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	return p.Run()
}

type Config struct {
	Command string

	RepoArg string

	BaseURL string
	APIKey  string
	Model   string

	AnthropicKey string
	GeminiKey    string

	RecentN   int
	MaxFiles  int
	Summarize bool

	Temperature float64
	Timeout     time.Duration // passed to TUI for AI request timeout

	DumpOutPath string

	InstructionsPath string

	// Config management
	ConfigPath string
	SaveConfig bool

	// Enhancements
	Conventional   bool
	Provider       string
	IgnoredFiles   []string
	HookFile       string
	PromptTemplate string
	ReviewLanguage string

	// Behavior modes
	Print        bool   // print message to stdout instead of launching TUI
	DryRun       bool   // generate/preview without committing
	Amend        bool   // amend the last commit instead of creating a new one
	Count        int    // number of candidate messages to generate
	ConfigAction string // subcommand for `config` (e.g. "show", "path")

	// Logging
	LogLevel  string
	LogOutput string
	LogFile   string
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Command == "config" {
		return runConfig(cfg)
	}
	if cfg.Command == "install-hook" {
		return InstallHook(ctx, cfg.RepoArg, cfg.Print)
	}
	if cfg.Command == "uninstall-hook" {
		return UninstallHook(ctx, cfg.RepoArg)
	}
	if cfg.Command == "ping" {
		return runPing(ctx, cfg)
	}
	if cfg.Command == "models" {
		return runModels(ctx, cfg)
	}

	repoRoot, err := gitx.ResolveRepoRoot(ctx, cfg.RepoArg)
	if err != nil {
		return logger.LogError(err, "failed to resolve repository root", "repo_arg", cfg.RepoArg)
	}

	customInstructions := ""
	if strings.TrimSpace(cfg.InstructionsPath) != "" {
		b, err := os.ReadFile(cfg.InstructionsPath)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		customInstructions = string(b)
	}

	// 1. Build Data
	data, err := buildPromptData(ctx, repoRoot, cfg.RecentN, cfg.MaxFiles, cfg.Summarize, customInstructions, cfg.IgnoredFiles)
	if err != nil {
		return logger.LogError(err, "failed to build prompt data")
	}
	data.SystemPromptTemplate = cfg.PromptTemplate
	data.ReviewLanguage = cfg.ReviewLanguage

	switch cfg.Command {
	case "dump-prompt":
		vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)
		return dumpPrompt(vscodeMsgs, cfg.DumpOutPath)

	case "suggest":
		provider, err := newProvider(cfg)
		if err != nil {
			return err
		}
		vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)

		// Non-interactive mode: generate once, print to stdout, optionally commit.
		if cfg.Print || cfg.DryRun {
			return runSuggestNonInteractive(ctx, cfg, repoRoot, provider, vscodeMsgs)
		}

		suggestTUI := newTuiModel(repoRoot, provider, vscodeMsgs, cfg.Temperature, cfg.Timeout, cfg.Conventional, cfg.HookFile)
		suggestTUI.amend = cfg.Amend
		suggestTUI.count = cfg.Count
		finalModel, err := runTUI(suggestTUI)
		if err != nil {
			return logger.LogError(err, "TUI execution failed")
		}

		if m, ok := finalModel.(tuiModel); ok {
			if m.err != nil {
				return logger.LogError(m.err, "TUI operation failed")
			}
		}
		return nil

	case "review":
		provider, err := newProvider(cfg)
		if err != nil {
			return err
		}
		reviewMsgs := vscodeprompt.BuildReviewMessages(data, true)
		finalModel, err := runTUI(newReviewModel(provider, reviewMsgs, cfg.Temperature, cfg.Timeout, true))
		if err != nil {
			return logger.LogError(err, "review TUI execution failed")
		}

		if m, ok := finalModel.(reviewModel); ok {
			if m.err != nil {
				return logger.LogError(m.err, "review operation failed")
			}
			// User selected "Suggest commit message" from review mode
			if m.switchToSuggest {
				vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)
				suggestTUI := newTuiModel(repoRoot, provider, vscodeMsgs, cfg.Temperature, cfg.Timeout, cfg.Conventional, cfg.HookFile)
				suggestTUI.amend = cfg.Amend
				suggestModel, err := runTUI(suggestTUI)
				if err != nil {
					return logger.LogError(err, "TUI execution failed")
				}
				if sm, ok := suggestModel.(tuiModel); ok {
					if sm.err != nil {
						return logger.LogError(sm.err, "TUI operation failed")
					}
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown -cmd=%s (use: suggest | review | dump-prompt | config | install-hook | uninstall-hook)", cfg.Command)
	}
}

func newProvider(cfg Config) (ai.Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, logger.LogError(ErrMissingModel, "model not configured")
	}

	switch strings.ToLower(cfg.Provider) {
	case "ollama":
		return ollama.New(ollama.Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		}), nil
	case "anthropic":
		if cfg.AnthropicKey == "" {
			return nil, logger.LogError(ErrMissingAPIKey, "anthropic key not configured")
		}
		return anthropic.New(anthropic.Config{
			APIKey: cfg.AnthropicKey,
			Model:  cfg.Model,
		}), nil
	case "gemini":
		if cfg.GeminiKey == "" {
			return nil, logger.LogError(ErrMissingAPIKey, "gemini key not configured")
		}
		return gemini.New(gemini.Config{
			APIKey: cfg.GeminiKey,
			Model:  cfg.Model,
		}), nil
	case "openai", "":
		if strings.TrimSpace(cfg.BaseURL) == "" && strings.TrimSpace(cfg.APIKey) == "" {
			return nil, logger.LogError(ErrMissingAPIKey, "openai api key not configured")
		}
		return openai.New(openai.Config{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		}), nil
	default:
		return nil, logger.LogError(fmt.Errorf("%w: %s", ErrUnknownProvider, cfg.Provider), "unsupported provider")
	}
}

func buildPromptData(ctx context.Context, repoRoot string, recentN, maxFiles int, summarize bool, customInstructions string, ignoredFiles []string) (vscodeprompt.Data, error) {
	recentN = clampNonNegative(recentN)
	if maxFiles <= 0 {
		maxFiles = 10
	}
	repoName := gitx.RepoNameFromRoot(repoRoot)

	branch, _ := gitx.CurrentBranch(ctx, repoRoot)
	userEmail, _ := gitx.GitConfig(ctx, repoRoot, "user.email")

	userCommits, _ := gitx.RecentCommitsByAuthor(ctx, repoRoot, recentN, userEmail)
	repoCommits, _ := gitx.RecentCommits(ctx, repoRoot, recentN)

	// Fetch more changes initially to account for filtering
	fetchFiles := maxFiles * 2
	if fetchFiles < 20 {
		fetchFiles = 20
	}
	changes, err := gitx.StagedChanges(ctx, repoRoot, fetchFiles)
	if err != nil {
		return vscodeprompt.Data{}, logger.LogError(err, "failed to get staged changes")
	}
	if len(changes) == 0 {
		return vscodeprompt.Data{}, logger.LogError(ErrNoStagedChanges, "no files staged for commit")
	}

	// Filter changes
	defaultIgnores := []string{
		"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"*.map", "*.svg", "*.min.js", "*.min.css",
	}
	// Combine ignores
	allIgnores := append(defaultIgnores, ignoredFiles...)

	filteredChanges := make([]vscodeprompt.Change, 0, maxFiles)
	for _, ch := range changes {
		if len(filteredChanges) >= maxFiles {
			break
		}

		// Check ignores
		if shouldIgnore(ch.Path, allIgnores) {
			// Maybe track skipped?
			continue
		}

		// Check size (simple heuristic: diff length)
		// Better: check file size if new, or diff size.
		// For simplicity, let's treat huge diffs as truncated.
		const maxDiffSize = 100 * 1024 // 100KB
		if len(ch.Diff) > maxDiffSize {
			ch.Diff = truncateUTF8(ch.Diff, 2000) + "\n...[Diff truncated due to size]..."
		}

		orig, _ := gitx.OriginalFileAtHEAD(ctx, repoRoot, ch.Path)
		if strings.TrimSpace(orig) == "" {
			// File might be new (not in HEAD yet), try reading from working tree
			orig, _ = gitx.ReadWorkingTreeFile(repoRoot, ch.Path)
		}

		// If original content is massive, truncate it too
		if len(orig) > maxDiffSize {
			orig = truncateUTF8(orig, 2000) + "\n...[Content truncated due to size]..."
		}

		attachment := vscodeprompt.BuildAttachment(repoRoot, ch.Path, orig, summarize)
		filteredChanges = append(filteredChanges, vscodeprompt.Change{
			Path:         ch.Path,
			Diff:         ch.Diff,
			OriginalCode: attachment,
		})
	}

	if len(filteredChanges) == 0 {
		return vscodeprompt.Data{}, fmt.Errorf("%w (checked %d files)", ErrAllFilesIgnored, len(changes))
	}

	return vscodeprompt.Data{
		RepositoryName:       repoName,
		BranchName:           branch,
		RecentUserCommits:    userCommits,
		RecentRepoCommits:    repoCommits,
		Changes:              filteredChanges,
		CustomInstructions:   customInstructions, // inserted into <custom-instructions>
		SummarizeAttachments: summarize,
	}, nil
}

// truncateUTF8 returns the first maxBytes bytes of s without splitting a
// multi-byte UTF-8 rune. The result is at most maxBytes bytes and always valid UTF-8.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// shouldIgnore reports whether filePath matches any of the ignore patterns.
// Supports exact matches, basename globs, full-path globs, and directory
// prefixes ("dir/" or "dir/**").
func shouldIgnore(filePath string, ignores []string) bool {
	p := filepath.ToSlash(filePath)
	base := filepath.Base(p)
	for _, ign := range ignores {
		ign = filepath.ToSlash(strings.TrimSpace(ign))
		if ign == "" {
			continue
		}
		// Exact match on full path or basename.
		if ign == p || ign == base {
			return true
		}
		// Directory prefix patterns.
		if strings.HasSuffix(ign, "/") && strings.HasPrefix(p, ign) {
			return true
		}
		if strings.HasSuffix(ign, "/**") {
			if strings.HasPrefix(p, strings.TrimSuffix(ign, "**")) {
				return true
			}
		}
		// Glob match on basename and on full path.
		if matched, _ := filepath.Match(ign, base); matched {
			return true
		}
		if matched, _ := filepath.Match(ign, p); matched {
			return true
		}
	}
	return false
}

// resolveConfigPath returns the effective config file path (defaults to
// ~/.commitgen.json when empty).
func resolveConfigPath(path string) string {
	if path != "" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".commitgen.json")
	}
	return "~/.commitgen.json"
}

// showConfig prints the saved configuration with secret values masked.
func showConfig(path string) error {
	fileCfg, err := config.Load(path)
	if err != nil {
		return logger.LogError(err, "failed to load config", "path", resolveConfigPath(path))
	}
	fileCfg.APIKey = maskSecret(fileCfg.APIKey)
	fileCfg.AnthropicKey = maskSecret(fileCfg.AnthropicKey)
	fileCfg.GeminiKey = maskSecret(fileCfg.GeminiKey)

	b, err := json.MarshalIndent(fileCfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("Config file: %s\n%s\n", resolveConfigPath(path), string(b))
	return nil
}

// maskSecret replaces a secret with a fixed-length mask, keeping it non-empty
// only when a value is present.
func maskSecret(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "********"
}

func runConfig(cfg Config) error {
	switch strings.ToLower(strings.TrimSpace(cfg.ConfigAction)) {
	case "path":
		fmt.Println(resolveConfigPath(cfg.ConfigPath))
		return nil
	case "show":
		return showConfig(cfg.ConfigPath)
	}

	newCfg, ok, err := runConfigInteractive(cfg)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Operation cancelled.")
		return nil
	}

	fileCfg := config.FileConfig{
		BaseURL:      newCfg.BaseURL,
		APIKey:       newCfg.APIKey,
		Model:        newCfg.Model,
		IgnoredFiles: newCfg.IgnoredFiles,

		RecentN:        &newCfg.RecentN,
		MaxFiles:       &newCfg.MaxFiles,
		Summarize:      &newCfg.Summarize,
		Temperature:    &newCfg.Temperature,
		Conventional:   &newCfg.Conventional,
		Provider:       newCfg.Provider,
		AnthropicKey:   newCfg.AnthropicKey,
		GeminiKey:      newCfg.GeminiKey,
		PromptTemplate: newCfg.PromptTemplate,
		ReviewLanguage: newCfg.ReviewLanguage,

		LogLevel:  newCfg.LogLevel,
		LogOutput: newCfg.LogOutput,
		LogFile:   newCfg.LogFile,
	}

	if err := config.Save(fileCfg, cfg.ConfigPath); err != nil {
		return logger.LogError(err, "failed to save config", "path", cfg.ConfigPath)
	}
	savedPath := cfg.ConfigPath
	if savedPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			savedPath = filepath.Join(home, ".commitgen.json")
		} else {
			savedPath = "~/.commitgen.json"
		}
	}
	fmt.Printf("\nConfiguration saved to %s\n", savedPath)
	return nil
}

func dumpPrompt(msgs []vscodeprompt.VSCodeMessage, outPath string) error {
	if strings.TrimSpace(outPath) == "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(msgs)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(msgs); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}
