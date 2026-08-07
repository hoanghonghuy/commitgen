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
	"unicode/utf8"

	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/analyzer"
	"github.com/hoanghonghuy/commitgen/internal/anthropic"
	"github.com/hoanghonghuy/commitgen/internal/config"
	"github.com/hoanghonghuy/commitgen/internal/gemini"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/ollama"
	"github.com/hoanghonghuy/commitgen/internal/openai"
	"github.com/hoanghonghuy/commitgen/internal/validator"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"

	tea "github.com/charmbracelet/bubbletea"
)

// runTUI runs a Bubble Tea program for the given model and returns the final
// model. It is a package-level variable so tests can substitute a stub instead
// of launching a real terminal program.
var runTUI = func(ctx context.Context, model tea.Model) (tea.Model, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	p := tea.NewProgram(model, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithMouseCellMotion())
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

	// Enhancements
	Conventional   bool
	Provider       string
	IgnoredFiles   []string
	HookFile       string
	PromptTemplate string
	ReviewLanguage string
	Locale         string // UI language (en, vi, ja, zh)
	RulesFile      string // path to validation rules file (.commitgen-rules.json)

	// PR generation
	BaseBranch string // target branch for merge-base (e.g. main); empty = auto-detect

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

	// Preserved from file config (not edited in the interactive form).
	PromptTemplateFile string
	TimeoutSeconds     *int
}

func Run(ctx context.Context, cfg Config) error {
	tr := i18n.New(i18n.Locale(cfg.Locale))

	if cfg.Command == "config" {
		return runConfig(cfg)
	}
	if cfg.Command == "install-hook" {
		return InstallHook(ctx, cfg.RepoArg, cfg.Print, cfg.ConfigPath, tr)
	}
	if cfg.Command == "uninstall-hook" {
		return UninstallHook(ctx, cfg.RepoArg, tr)
	}
	if cfg.Command == "ping" {
		return runPing(ctx, cfg, tr)
	}
	if cfg.Command == "models" {
		return runModels(ctx, cfg, tr)
	}

	repoRoot, err := gitx.ResolveRepoRoot(ctx, cfg.RepoArg)
	if err != nil {
		return logger.LogError(err, "failed to resolve repository root", "repo_arg", cfg.RepoArg)
	}
	if cfg.Command == "style" {
		return runStyle(ctx, repoRoot, cfg.RecentN)
	}

	customInstructions := ""
	if strings.TrimSpace(cfg.InstructionsPath) != "" {
		b, err := os.ReadFile(cfg.InstructionsPath)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		customInstructions = string(b)
	}

	if cfg.Command == "pr" {
		provider, err := newProvider(cfg)
		if err != nil {
			return err
		}
		data, err := buildPRPromptData(ctx, repoRoot, cfg.BaseBranch, cfg.RecentN, cfg.MaxFiles, cfg.Summarize, customInstructions, cfg.IgnoredFiles)
		if err != nil {
			if errors.Is(err, ErrNoPRChanges) {
				return fmt.Errorf("%s", tr.T("error.no_pr_changes"))
			}
			return logger.LogError(err, "failed to build PR prompt data")
		}
		data.SystemPromptTemplate = cfg.PromptTemplate
		data.ReviewLanguage = cfg.ReviewLanguage
		return runPR(ctx, cfg, repoRoot, provider, data, tr)
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
		v := resolveValidator(cfg, repoRoot, tr)
		provider, err := newProvider(cfg)
		if err != nil {
			return err
		}
		vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)

		// Non-interactive mode: generate once and print to stdout. With --print
		// it also writes the hook file when configured; it never creates a commit.
		if shouldRunSuggestHeadless(cfg) {
			return runSuggestNonInteractive(ctx, cfg, repoRoot, provider, vscodeMsgs, v, tr)
		}

		suggestTUI := newTuiModel(ctx, repoRoot, provider, vscodeMsgs, cfg.Temperature, cfg.Timeout, cfg.Conventional, cfg.HookFile, tr, v)
		suggestTUI.amend = cfg.Amend
		suggestTUI.count = cfg.Count
		finalModel, err := runTUI(ctx, suggestTUI)
		if err != nil {
			return logger.LogError(err, "TUI execution failed")
		}

		if m, ok := finalModel.(tuiModel); ok {
			if m.reachedDone {
				printDurableOutcome(os.Stderr, tr, m.err)
			}
			if m.err != nil {
				return logger.LogError(m.err, "TUI operation failed")
			}
		}
		return nil

	case "review":
		v := resolveValidator(cfg, repoRoot, tr)
		provider, err := newProvider(cfg)
		if err != nil {
			return err
		}
		reviewMsgs := vscodeprompt.BuildReviewMessages(data, true)
		reviewTUI := newReviewModel(ctx, provider, reviewMsgs, cfg.Temperature, cfg.Timeout, true, tr)
		// Pre-build the full-review system message so "View Details" honors a
		// custom prompt template instead of always using the built-in default.
		if fullMsgs := vscodeprompt.BuildReviewMessages(data, false); len(fullMsgs) >= 1 {
			fullSystem := fullMsgs[0]
			reviewTUI.fullReviewSystem = &fullSystem
		}
		finalModel, err := runTUI(ctx, reviewTUI)
		if err != nil {
			return logger.LogError(err, "review TUI execution failed")
		}

		if m, ok := finalModel.(reviewModel); ok {
			if m.reachedDone {
				printDurableOutcome(os.Stderr, tr, m.err)
			}
			if m.err != nil {
				return logger.LogError(m.err, "review operation failed")
			}
			// User selected "Suggest commit message" from review mode
			if m.switchToSuggest {
				vscodeMsgs := vscodeprompt.BuildVSCodeMessages(data)
				suggestTUI := newTuiModel(ctx, repoRoot, provider, vscodeMsgs, cfg.Temperature, cfg.Timeout, cfg.Conventional, cfg.HookFile, tr, v)
				suggestTUI.amend = cfg.Amend
				suggestTUI.count = cfg.Count
				suggestModel, err := runTUI(ctx, suggestTUI)
				if err != nil {
					return logger.LogError(err, "TUI execution failed")
				}
				if sm, ok := suggestModel.(tuiModel); ok {
					if sm.reachedDone {
						printDurableOutcome(os.Stderr, tr, sm.err)
					}
					if sm.err != nil {
						return logger.LogError(sm.err, "TUI operation failed")
					}
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown -cmd=%s (use: suggest | review | pr | dump-prompt | config | install-hook | uninstall-hook | ping | models | style | version)", cfg.Command)
	}
}

// resolveValidator loads validation rules when a rules file is configured or
// auto-discovered at <repo>/.commitgen-rules.json. Returns nil when validation
// is disabled (no rules file).
func resolveValidator(cfg Config, repoRoot string, tr *i18n.Translator) *validator.Validator {
	rulesPath := strings.TrimSpace(cfg.RulesFile)
	if rulesPath == "" {
		candidate := filepath.Join(repoRoot, ".commitgen-rules.json")
		if _, err := os.Stat(candidate); err == nil {
			rulesPath = candidate
		}
	} else if !filepath.IsAbs(rulesPath) {
		rulesPath = filepath.Join(repoRoot, rulesPath)
	}
	if rulesPath == "" {
		return nil
	}
	rulesCfg, err := loadRulesConfig(rulesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", tr.T("warn.rules_file", rulesPath, err))
		return nil
	}
	v := validator.New(*rulesCfg)
	if !v.Enabled() {
		return nil
	}
	return v
}

// loadRulesConfig reads a JSON rules configuration file and merges it with defaults.
func loadRulesConfig(path string) (*validator.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg validator.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse rules config: %w", err)
	}
	return validator.LoadConfig(&cfg), nil
}

func newProvider(cfg Config) (ai.Provider, error) {
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, logger.LogError(ErrMissingModel, "model not configured")
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "ollama", "ollama-cloud":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = ollama.ResolveBaseURL(cfg.BaseURL, cfg.APIKey)
		}
		if (strings.EqualFold(cfg.Provider, "ollama-cloud") || ollama.IsCloudBaseURL(baseURL)) && strings.TrimSpace(cfg.APIKey) == "" {
			return nil, logger.LogError(ErrMissingAPIKey, "ollama cloud api key not configured")
		}
		return ollama.New(ollama.Config{
			BaseURL: baseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
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
	case "openai", "openrouter", "compatible", "":
		if strings.TrimSpace(cfg.BaseURL) == "" && strings.TrimSpace(cfg.APIKey) == "" {
			return nil, logger.LogError(ErrMissingAPIKey, "openai api key not configured")
		}
		return openai.New(openai.Config{
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		}), nil
	default:
		return nil, logger.LogError(&unknownProviderError{Provider: cfg.Provider}, "unsupported provider")
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
	styleGuidance := analyzer.AnalyzeCommitStyle(repoCommits).Guidance()
	commitTemplate, err := gitx.GetCommitTemplate(ctx, repoRoot)
	if err != nil {
		logger.Warn("failed to read git commit template", "error", err)
	}

	stagedFiles, err := gitx.StagedFileNames(ctx, repoRoot)
	if err != nil {
		return vscodeprompt.Data{}, logger.LogError(err, "failed to get staged changes")
	}
	if len(stagedFiles) == 0 {
		return vscodeprompt.Data{}, logger.LogError(ErrNoStagedChanges, "no files staged for commit")
	}

	// Filter changes — scan all staged files so ignored entries at the front
	// do not hide valid files further down the list.
	defaultIgnores := []string{
		"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"*.map", "*.svg", "*.min.js", "*.min.css",
	}
	allIgnores := append(defaultIgnores, ignoredFiles...)

	filteredChanges := make([]vscodeprompt.Change, 0, maxFiles)
	for _, path := range stagedFiles {
		if len(filteredChanges) >= maxFiles {
			break
		}
		if shouldIgnore(path, allIgnores) {
			continue
		}

		diff, _ := gitx.Git(ctx, repoRoot, "diff", "--staged", "--", path)
		ch := vscodeprompt.Change{Path: path, Diff: diff}

		const maxDiffSize = 100 * 1024 // 100KB
		if len(ch.Diff) > maxDiffSize {
			ch.Diff = truncateUTF8(ch.Diff, 2000) + "\n...[Diff truncated due to size]..."
		}

		orig, _ := gitx.OriginalFileAtHEAD(ctx, repoRoot, ch.Path)
		if strings.TrimSpace(orig) == "" {
			orig, _ = gitx.ReadWorkingTreeFile(repoRoot, ch.Path)
		}
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
		return vscodeprompt.Data{}, &AllFilesIgnoredError{Checked: len(stagedFiles)}
	}

	return vscodeprompt.Data{
		RepositoryName:       repoName,
		BranchName:           branch,
		RecentUserCommits:    userCommits,
		RecentRepoCommits:    repoCommits,
		CommitTemplate:       commitTemplate,
		CommitStyleGuidance:  styleGuidance,
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
	tr := i18n.New(i18n.Locale(firstNonEmpty(fileCfg.Locale, "en")))
	resolvedPath := resolveConfigPath(path)
	fmt.Printf("%s\n", tr.T("config.file_path", resolvedPath))
	fmt.Printf("%s\n", tr.T("config.show.provider", providerConfigLabel(fileCfg.Provider, fileCfg.CompatibleBaseURL, config.APIKeyFor(fileCfg, fileCfg.Provider))))

	if fileCfg.APIKeys != nil {
		masked := make(map[string]string, len(fileCfg.APIKeys))
		for k, v := range fileCfg.APIKeys {
			masked[k] = maskSecret(v)
		}
		fileCfg.APIKeys = masked
	}
	fileCfg.APIKey = maskSecret(fileCfg.APIKey)
	fileCfg.AnthropicKey = maskSecret(fileCfg.AnthropicKey)
	fileCfg.GeminiKey = maskSecret(fileCfg.GeminiKey)

	b, err := json.MarshalIndent(fileCfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

const secretMask = "********"

// maskSecret replaces a secret with a fixed-length mask, keeping it non-empty
// only when a value is present.
func maskSecret(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return secretMask
}

// preserveSecret keeps the existing secret when the form submits an empty value
// or the display mask (********).
func preserveSecret(submitted, existing string) string {
	s := strings.TrimSpace(submitted)
	if s == "" || s == secretMask {
		return existing
	}
	return s
}

func runConfig(cfg Config) error {
	tr := i18n.New(i18n.Locale(cfg.Locale))
	switch strings.ToLower(strings.TrimSpace(cfg.ConfigAction)) {
	case "path":
		fmt.Println(resolveConfigPath(cfg.ConfigPath))
		return nil
	case "show":
		return showConfig(cfg.ConfigPath)
	}

	savePath := resolveConfigPath(cfg.ConfigPath)
	if cfg.ConfigPath == "" {
		if local, ok := config.RepoLocalConfigPath(); ok {
			fmt.Fprintf(os.Stderr, "%s\n", tr.T("config.repo_overlay_warn", local, savePath))
		}
	}

	newCfg, submittedKey, ok, err := runConfigInteractive(cfg, savePath, tr)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println(tr.T("config.cancelled"))
		return nil
	}

	existing, _ := config.Load(cfg.ConfigPath)
	fileCfg := fileConfigFromInteractive(
		newCfg,
		existing,
		submittedKey,
		newCfg.RecentN,
		newCfg.MaxFiles,
		newCfg.Temperature,
		newCfg.Summarize,
		newCfg.Conventional,
		newCfg.TimeoutSeconds,
	)

	if err := config.Save(fileCfg, cfg.ConfigPath); err != nil {
		return logger.LogError(err, "failed to save config", "path", savePath)
	}
	fmt.Printf("\n%s\n", tr.T("config.saved", savePath))
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
