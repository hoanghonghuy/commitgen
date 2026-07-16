package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/hoanghonghuy/commitgen/internal/config"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

// runConfigInteractive launches a two-step TUI: provider select, then fields.
// Aborting step 2 returns to step 1 (back) instead of exiting.
func runConfigInteractive(cfg Config, savePath string, tr *i18n.Translator) (Config, string, bool, error) {
	existing, _ := config.Load(cfg.ConfigPath)

	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = existing.Provider
	}
	if provider == "" {
		provider = config.ProviderOpenAI
	}
	// Map legacy ollama+url to new ids when opening the form.
	if provider == "ollama" {
		if config.APIKeyFor(existing, config.ProviderOllamaCloud) != "" || strings.Contains(strings.ToLower(cfg.BaseURL), "ollama.com") {
			provider = config.ProviderOllamaCloud
		}
	}

	model := cfg.Model
	if model == "" {
		model = existing.Model
	}
	apiKey := maskSecret(config.APIKeyFor(existing, provider))
	baseURL := existing.CompatibleBaseURL
	if baseURL == "" && step2IncludesBaseURL(provider) {
		baseURL = cfg.BaseURL
	}

	promptTemplate := cfg.PromptTemplate
	rulesFile := cfg.RulesFile
	recentNStr := fmt.Sprintf("%d", cfg.RecentN)
	maxFilesStr := fmt.Sprintf("%d", cfg.MaxFiles)
	tempStr := fmt.Sprintf("%.2f", cfg.Temperature)
	summarize := cfg.Summarize
	conventional := cfg.Conventional
	ignoredFilesStr := strings.Join(cfg.IgnoredFiles, ", ")

	reviewLanguage := cfg.ReviewLanguage
	if reviewLanguage == "" {
		reviewLanguage = "en"
	}
	locale := cfg.Locale
	if locale == "" {
		locale = "en"
	}
	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	logOutput := cfg.LogOutput
	if logOutput == "" {
		logOutput = "both"
	}
	logFile := cfg.LogFile

	timeoutStr := "120"
	if cfg.TimeoutSeconds != nil && *cfg.TimeoutSeconds > 0 {
		timeoutStr = fmt.Sprintf("%d", *cfg.TimeoutSeconds)
	} else if cfg.Timeout > 0 {
		timeoutStr = fmt.Sprintf("%d", int(cfg.Timeout.Seconds()))
	}

	for {
		step1 := applyFormAccessibility(huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title(tr.T("config.form.title")).
					Description(tr.T("config.form_intro") + "\n" + tr.T("config.save_target", savePath)),
				huh.NewSelect[string]().
					Title(tr.T("config.field.provider")).
					Options(
						huh.NewOption(tr.T("config.provider.openai"), config.ProviderOpenAI),
						huh.NewOption(tr.T("config.provider.openrouter"), config.ProviderOpenRouter),
						huh.NewOption(tr.T("config.provider.compatible"), config.ProviderCompatible),
						huh.NewOption(tr.T("config.provider.ollama_local"), config.ProviderOllama),
						huh.NewOption(tr.T("config.provider.ollama_cloud"), config.ProviderOllamaCloud),
						huh.NewOption(tr.T("config.provider.anthropic"), config.ProviderAnthropic),
						huh.NewOption(tr.T("config.provider.gemini"), config.ProviderGemini),
					).
					Value(&provider),
			),
		), os.Getenv)
		if err := step1.Run(); err != nil {
			return cfg, "", false, err
		}

		// Refresh masked key when provider changes.
		apiKey = maskSecret(config.APIKeyFor(existing, provider))
		if baseURL == "" && step2IncludesBaseURL(provider) {
			baseURL = existing.CompatibleBaseURL
			if baseURL == "" {
				baseURL = cfg.BaseURL
			}
		}

		aiFields := []huh.Field{
			huh.NewInput().
				Title(tr.T("config.field.model")).
				Description(tr.T("config.field.model.desc")).
				Suggestions(config.ModelSuggestions(provider)).
				Value(&model),
			huh.NewInput().
				Title(tr.T("config.field.api_key")).
				Description(tr.T("config.field.api_key.desc")).
				Value(&apiKey).
				EchoMode(huh.EchoModePassword),
		}
		if step2IncludesBaseURL(provider) {
			aiFields = append(aiFields,
				huh.NewInput().
					Title(tr.T("config.field.base_url")).
					Description(tr.T("config.field.base_url.desc")).
					Placeholder(tr.T("config.field.base_url.placeholder")).
					Value(&baseURL),
			)
		}
		aiFields = append(aiFields,
			huh.NewInput().
				Title(tr.T("config.field.prompt_template")).
				Description(tr.T("config.field.prompt_template.desc")).
				Value(&promptTemplate),
		)

		step2 := applyFormAccessibility(huh.NewForm(
			huh.NewGroup(aiFields...),
			huh.NewGroup(
				huh.NewInput().
					Title(tr.T("config.field.recent_n")).
					Description(tr.T("config.field.recent_n.desc")).
					Value(&recentNStr).
					Validate(func(s string) error { return validateIntField(tr, s) }),
				huh.NewInput().
					Title(tr.T("config.field.max_files")).
					Description(tr.T("config.field.max_files.desc")).
					Value(&maxFilesStr).
					Validate(func(s string) error { return validateIntField(tr, s) }),
				huh.NewInput().
					Title(tr.T("config.field.temperature")).
					Description(tr.T("config.field.temperature.desc")).
					Value(&tempStr).
					Validate(func(s string) error {
						v, err := strconv.ParseFloat(s, 64)
						if err != nil {
							return fmt.Errorf("%s", tr.T("config.field.integer.error"))
						}
						if v < 0 || v > 2.0 {
							return fmt.Errorf("%s", tr.T("config.field.temperature.error"))
						}
						return nil
					}),
				huh.NewInput().
					Title(tr.T("config.field.timeout")).
					Description(tr.T("config.field.timeout.desc")).
					Value(&timeoutStr).
					Validate(func(s string) error {
						if err := validateIntField(tr, s); err != nil {
							return err
						}
						v, _ := strconv.Atoi(strings.TrimSpace(s))
						if v <= 0 {
							return fmt.Errorf("%s", tr.T("config.field.timeout.error"))
						}
						return nil
					}),
			),
			huh.NewGroup(
				huh.NewConfirm().
					Title(tr.T("config.field.summarize")).
					Description(tr.T("config.field.summarize.desc")).
					Value(&summarize),
				huh.NewConfirm().
					Title(tr.T("config.field.conventional")).
					Description(tr.T("config.field.conventional.desc")).
					Value(&conventional),
			),
			huh.NewGroup(
				huh.NewInput().
					Title(tr.T("config.field.ignored_files")).
					Description(tr.T("config.field.ignored_files.desc")).
					Value(&ignoredFilesStr),
				huh.NewInput().
					Title(tr.T("config.field.rules_file")).
					Description(tr.T("config.field.rules_file.desc")).
					Value(&rulesFile),
			),
			huh.NewGroup(
				huh.NewSelect[string]().
					Title(tr.T("config.field.ui_language")).
					Description(tr.T("config.field.ui_language.desc")).
					Options(
						huh.NewOption(tr.T("config.locale.auto"), "auto"),
						huh.NewOption("English", "en"),
						huh.NewOption("Tiếng Việt", "vi"),
						huh.NewOption("日本語", "ja"),
						huh.NewOption("中文", "zh"),
					).
					Value(&locale),
			),
			huh.NewGroup(
				huh.NewSelect[string]().
					Title(tr.T("config.field.review_language")).
					Description(tr.T("config.field.review_language.desc")).
					Options(
						huh.NewOption("English", "en"),
						huh.NewOption("Tiếng Việt", "vi"),
						huh.NewOption("日本語", "ja"),
						huh.NewOption("中文", "zh"),
					).
					Value(&reviewLanguage),
			),
			huh.NewGroup(
				huh.NewNote().
					Title(tr.T("config.advanced.title")).
					Description(tr.T("config.advanced.desc")),
				huh.NewNote().
					Title(tr.T("config.logging.title")).
					Description(tr.T("config.logging.desc")),
				huh.NewSelect[string]().
					Title(tr.T("config.field.log_level")).
					Options(
						huh.NewOption(tr.T("config.log_level.debug"), "debug"),
						huh.NewOption(tr.T("config.log_level.info"), "info"),
						huh.NewOption(tr.T("config.log_level.warn"), "warn"),
						huh.NewOption(tr.T("config.log_level.error"), "error"),
					).
					Value(&logLevel),
				huh.NewSelect[string]().
					Title(tr.T("config.field.log_output")).
					Options(
						huh.NewOption(tr.T("config.log_output.stderr"), "stderr"),
						huh.NewOption(tr.T("config.log_output.file"), "file"),
						huh.NewOption(tr.T("config.log_output.both"), "both"),
					).
					Value(&logOutput),
				huh.NewInput().
					Title(tr.T("config.field.log_file")).
					Description(tr.T("config.field.log_file.desc")).
					Value(&logFile),
			),
		), os.Getenv)

		if err := step2.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				continue // back to provider selection
			}
			return cfg, "", false, err
		}

		cfg.Provider = provider
		cfg.Model = model
		cfg.BaseURL = baseURL
		cfg.PromptTemplate = promptTemplate
		cfg.RulesFile = rulesFile

		if v, err := strconv.Atoi(recentNStr); err == nil {
			cfg.RecentN = v
		}
		if v, err := strconv.Atoi(maxFilesStr); err == nil {
			cfg.MaxFiles = v
		}
		if v, err := strconv.ParseFloat(tempStr, 64); err == nil {
			cfg.Temperature = v
		}
		if v, err := strconv.Atoi(timeoutStr); err == nil && v > 0 {
			cfg.TimeoutSeconds = &v
			cfg.Timeout = time.Duration(v) * time.Second
		}
		cfg.Summarize = summarize
		cfg.Conventional = conventional

		rawIgnores := strings.Split(ignoredFilesStr, ",")
		var ignores []string
		for _, s := range rawIgnores {
			s = strings.TrimSpace(s)
			if s != "" {
				ignores = append(ignores, s)
			}
		}
		cfg.IgnoredFiles = ignores
		cfg.ReviewLanguage = reviewLanguage
		cfg.Locale = locale
		cfg.LogLevel = logLevel
		cfg.LogOutput = logOutput
		cfg.LogFile = logFile

		return cfg, apiKey, true, nil
	}
}
