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
	"github.com/hoanghonghuy/commitgen/internal/ollama"
	"github.com/hoanghonghuy/commitgen/internal/openai"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"

	"github.com/briandowns/spinner"
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
	Timeout     time.Duration // assigned in main, not used here directly

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
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Command == "config" {
		return runConfig(cfg)
	}
	if cfg.Command == "install-hook" {
		return InstallHook(ctx)
	}

	repoRoot, err := gitx.ResolveRepoRoot(ctx, cfg.RepoArg)
	if err != nil {
		return err
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
		return err
	}
	data.SystemPromptTemplate = cfg.PromptTemplate

	vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)

	switch cfg.Command {
	case "dump-prompt":
		return dumpPrompt(vscodeMsgs, cfg.DumpOutPath)

	case "suggest":
		if strings.TrimSpace(cfg.Model) == "" {
			return errors.New("missing model. Set flags or env COMMITAI_MODEL")
		}

		var provider ai.Provider

		switch strings.ToLower(cfg.Provider) {
		case "ollama":
			provider = ollama.New(ollama.Config{
				BaseURL: cfg.BaseURL,
				Model:   cfg.Model,
			})
		case "anthropic":
			if cfg.AnthropicKey == "" {
				return errors.New("missing anthropic key. Set flags or env COMMITAI_ANTHROPIC_KEY")
			}
			provider = anthropic.New(anthropic.Config{
				APIKey: cfg.AnthropicKey,
				Model:  cfg.Model,
			})
		case "gemini":
			if cfg.GeminiKey == "" {
				return errors.New("missing gemini key. Set flags or env COMMITAI_GEMINI_KEY")
			}
			provider = gemini.New(gemini.Config{
				APIKey: cfg.GeminiKey,
				Model:  cfg.Model,
			})
		case "openai", "":
			if strings.TrimSpace(cfg.BaseURL) == "" && strings.TrimSpace(cfg.APIKey) == "" {
				// Warn or error? OpenAI usually needs Key.
				// But let's assume if BaseURL is set (e.g. local compatible), Key might be optional?
				// For OpenAI official, Key is required.
			}
			provider = openai.New(openai.Config{
				BaseURL: cfg.BaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.Model,
			})
		default:
			return fmt.Errorf("unknown provider: %s (supported: openai, ollama, anthropic, gemini)", cfg.Provider)
		}

		return runInteractiveLoop(ctx, repoRoot, provider, vscodeMsgs, cfg.Temperature, cfg.Conventional, cfg.HookFile)

	default:
		return fmt.Errorf("unknown -cmd=%s (use suggest | dump-prompt | config)", cfg.Command)
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
		return vscodeprompt.Data{}, err
	}
	if len(changes) == 0 {
		return vscodeprompt.Data{}, errors.New("no staged changes. Run: git add -A")
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

func runInteractiveLoop(ctx context.Context, repoRoot string, provider ai.Provider, initialMsgs []vscodeprompt.VSCodeMessage, temp float64, conventional bool, hookFile string) error {
	msgs := initialMsgs

	for {
		// Spinner
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Generating commit message..."
		s.Start()

		// Add Conventional Commits instruction if requested
		// We append this every time to ensure it's at the end of context if we were aggregating,
		// but here we are sending fresh request mostly.
		// However, vscodeprompt messages are fixed.
		// If we want to enforce it, we should add it to the messages being sent.
		// Since we want to support regeneration, let's copy the base messages.
		currentMsgs := make([]vscodeprompt.VSCodeMessage, len(msgs))
		copy(currentMsgs, msgs)

		if conventional {
			reminderMsg := vscodeprompt.VSCodeMessage{
				Role: 1, // user
				Content: []vscodeprompt.VSCodeContentPart{
					{Type: 1, Text: "CRITICAL INSTRUCTION: You must strictly follow the Conventional Commits specification (e.g. 'feat: add spinner', 'fix: resolve bug').\nDo not just describe the change; prefix it with the type."},
				},
			}
			currentMsgs = append(currentMsgs, reminderMsg)
		}

		var commitMsgRaw string
		var err error
		maxRetries := 5

		for i := 0; i < maxRetries; i++ {
			commitMsgRaw, err = provider.GenerateCommitMessage(ctx, currentMsgs, temp)
			if err == nil {
				break
			}
			// Check for specific error to retry
			if strings.Contains(err.Error(), "empty choices") {
				if i < maxRetries-1 {
					// Stop spinner to print message
					s.Stop()
					fmt.Printf("\n⚠️  Provider returned no choices. Retrying (%d/%d)...\n", i+1, maxRetries-1)
					s.Start()
					time.Sleep(500 * time.Millisecond)
					continue
				}
			}
			// Propagate other errors or if retries exhausted
			break
		}
		s.Stop() // Stop spinner

		if err != nil {
			return err
		}

		commitMsg, ok := vscodeprompt.ExtractOneTextCodeBlock(commitMsgRaw)
		if !ok {
			fmt.Fprintln(os.Stderr, "Warning: model formatting issue (raw output shown below)")
			commitMsg = commitMsgRaw
		}

		// Inner Confirmation Loop
		for {
			action, err := confirmCommitInteractive(commitMsg)
			if err != nil {
				return err
			}

			switch action {
			case ActionCommit:
				if hookFile != "" {
					// Hook mode: Write to file instead of running git commit
					if err := os.WriteFile(hookFile, []byte(commitMsg), 0644); err != nil {
						return fmt.Errorf("write hook file: %w", err)
					}
					fmt.Println("Message generated for git hook.")
					return nil
				}
				return gitx.Commit(ctx, repoRoot, commitMsg)

			case ActionEdit:
				newMsg, err := editCommitMessageInteractive(commitMsg)
				if err != nil {
					return err
				}
				commitMsg = newMsg
				// Stay in confirmation loop to approve the new message
				continue

			case ActionRegenerate:
				fmt.Println("Regenerating...")
				// Break inner loop to continue outer loop
				goto NextGeneration

			case ActionCancel:
				fmt.Println("Cancelled.")
				if hookFile != "" {
					return fmt.Errorf("commit cancelled by user")
				}
				return nil
			}
		}
	NextGeneration:
	}
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
	}

	if err := config.Save(fileCfg, cfg.ConfigPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
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
