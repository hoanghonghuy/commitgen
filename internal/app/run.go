package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// Logging
	LogLevel  string
	LogOutput string
	LogFile   string
}

func Run(ctx context.Context, cfg Config) error {
	logger.Debug("app.Run started", "command", cfg.Command)
	
	if cfg.Command == "config" {
		return runConfig(cfg)
	}
	if cfg.Command == "install-hook" {
		return InstallHook()
	}
	if cfg.Command == "uninstall-hook" {
		return UninstallHook()
	}

	repoRoot, err := gitx.ResolveRepoRoot(ctx, cfg.RepoArg)
	if err != nil {
		return logger.LogError(err, "failed to resolve repository root", "repo_arg", cfg.RepoArg)
	}
	logger.Debug("repository resolved", "repo_root", repoRoot)

	customInstructions := ""
	if strings.TrimSpace(cfg.InstructionsPath) != "" {
		b, err := os.ReadFile(cfg.InstructionsPath)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		customInstructions = string(b)
	}

	// 1. Build Data
	logger.Debug("building prompt data", "recent_n", cfg.RecentN, "max_files", cfg.MaxFiles)
	data, err := buildPromptData(ctx, repoRoot, cfg.RecentN, cfg.MaxFiles, cfg.Summarize, customInstructions, cfg.IgnoredFiles)
	if err != nil {
		return logger.LogError(err, "failed to build prompt data")
	}
	data.SystemPromptTemplate = cfg.PromptTemplate

	vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)

	switch cfg.Command {
	case "dump-prompt":
		return dumpPrompt(vscodeMsgs, cfg.DumpOutPath)

	case "suggest":
		logger.Info("starting commit message suggestion", "provider", cfg.Provider, "model", cfg.Model)
		if strings.TrimSpace(cfg.Model) == "" {
			return logger.LogError(errors.New("missing model"), "model not configured")
		}

		var provider ai.Provider

		switch strings.ToLower(cfg.Provider) {
		case "ollama":
			logger.Debug("using ollama provider", "base_url", cfg.BaseURL)
			provider = ollama.New(ollama.Config{
				BaseURL: cfg.BaseURL,
				Model:   cfg.Model,
			})
		case "anthropic":
			if cfg.AnthropicKey == "" {
				return logger.LogError(errors.New("missing anthropic key"), "anthropic key not configured")
			}
			logger.Debug("using anthropic provider")
			provider = anthropic.New(anthropic.Config{
				APIKey: cfg.AnthropicKey,
				Model:  cfg.Model,
			})
		case "gemini":
			if cfg.GeminiKey == "" {
				return logger.LogError(errors.New("missing gemini key"), "gemini key not configured")
			}
			logger.Debug("using gemini provider")
			provider = gemini.New(gemini.Config{
				APIKey: cfg.GeminiKey,
				Model:  cfg.Model,
			})
		case "openai", "":
			if strings.TrimSpace(cfg.BaseURL) == "" && strings.TrimSpace(cfg.APIKey) == "" {
				return logger.LogError(errors.New("missing api key"), "openai api key not configured")
			}
			logger.Debug("using openai provider", "base_url", cfg.BaseURL)
			provider = openai.New(openai.Config{
				BaseURL: cfg.BaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.Model,
			})
		default:
			return logger.LogError(fmt.Errorf("unknown provider: %s", cfg.Provider), "unsupported provider")
		}

		p := tea.NewProgram(
			newTuiModel(repoRoot, provider, vscodeMsgs, cfg.Temperature, cfg.Timeout, cfg.Conventional, cfg.HookFile),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)
		logger.Debug("starting TUI")
		_, err = p.Run()
		if err != nil {
			return logger.LogError(err, "TUI execution failed")
		}
		return err

	default:
		return fmt.Errorf("unknown -cmd=%s (use: suggest | dump-prompt | config | install-hook | uninstall-hook)", cfg.Command)
	}
}

func buildPromptData(ctx context.Context, repoRoot string, recentN, maxFiles int, summarize bool, customInstructions string, ignoredFiles []string) (vscodeprompt.Data, error) {
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
		return vscodeprompt.Data{}, logger.LogError(errors.New("no staged changes"), "no files staged for commit")
	}
	logger.Debug("staged changes retrieved", "count", len(changes))

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
			ch.Diff = ch.Diff[:2000] + "\n...[Diff truncated due to size]..."
		}

		orig, _ := gitx.OriginalFileAtHEAD(ctx, repoRoot, ch.Path)
		if strings.TrimSpace(orig) == "" {
			orig, _ = gitx.ReadWorkingTreeFile(repoRoot, ch.Path)
		}

		// If original content is massive, truncate it too
		if len(orig) > maxDiffSize {
			orig = orig[:2000] + "\n...[Content truncated due to size]..."
		}

		attachment := vscodeprompt.BuildAttachment(repoRoot, ch.Path, orig, summarize)
		filteredChanges = append(filteredChanges, vscodeprompt.Change{
			Path:         ch.Path,
			Diff:         ch.Diff,
			OriginalCode: attachment,
		})
	}

	if len(filteredChanges) == 0 {
		return vscodeprompt.Data{}, fmt.Errorf("all staged files were ignored (checked %d files)", len(changes))
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

func shouldIgnore(pattern string, ignores []string) bool {
	base := filepath.Base(pattern)
	for _, ign := range ignores {
		// Simple equality
		if ign == base || ign == pattern {
			return true
		}
		// Glob match
		if matched, _ := filepath.Match(ign, base); matched {
			return true
		}
	}
	return false
}


func runConfig(cfg Config) error {
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
		
		LogLevel:  newCfg.LogLevel,
		LogOutput: newCfg.LogOutput,
		LogFile:   newCfg.LogFile,
	}

	if err := config.Save(fileCfg, cfg.ConfigPath); err != nil {
		return logger.LogError(err, "failed to save config", "path", cfg.ConfigPath)
	}
	logger.Info("configuration saved", "path", cfg.ConfigPath)
	fmt.Printf("\nConfiguration saved to %s\n", cfg.ConfigPath)
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
