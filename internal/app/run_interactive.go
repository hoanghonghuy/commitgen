package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

// runConfigInteractive launches a TUI form to edit key config fields
func runConfigInteractive(cfg Config, savePath string, tr *i18n.Translator) (Config, bool, error) {
	baseURL := cfg.BaseURL
	apiKey := cfg.APIKey
	anthropicKey := cfg.AnthropicKey
	geminiKey := cfg.GeminiKey
	model := cfg.Model
	promptTemplate := cfg.PromptTemplate
	rulesFile := cfg.RulesFile
	provider := cfg.Provider
	if provider == "" {
		provider = "openai"
	}
	provider = ollamaOptionForConfig(provider, cfg.BaseURL, cfg.APIKey)

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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(tr.T("config.form.title")).
				Description(tr.T("config.form_intro") + "\n" + tr.T("config.save_target", savePath)),

			huh.NewSelect[string]().
				Title(tr.T("config.field.provider")).
				Options(
					huh.NewOption(tr.T("config.provider.openai"), "openai"),
					huh.NewOption(tr.T("config.provider.ollama_local"), ollamaLocalOption),
					huh.NewOption(tr.T("config.provider.ollama_cloud"), ollamaCloudOption),
					huh.NewOption(tr.T("config.provider.anthropic"), "anthropic"),
					huh.NewOption(tr.T("config.provider.gemini"), "gemini"),
				).
				Value(&provider),

			huh.NewInput().
				Title(tr.T("config.field.base_url")).
				Description(tr.T("config.field.base_url.desc")).
				Placeholder(tr.T("config.field.base_url.placeholder")).
				Suggestions([]string{
					"https://api.openai.com/v1",
					"https://openrouter.ai/api/v1",
					"https://api.mistral.ai/v1",
					"https://ollama.com",
					"http://localhost:11434",
				}).
				Value(&baseURL),

			huh.NewInput().
				Title(tr.T("config.field.api_key")).
				Description(tr.T("config.field.api_key.desc")).
				Value(&apiKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title(tr.T("config.field.anthropic_key")).
				Description(tr.T("config.field.anthropic_key.desc")).
				Value(&anthropicKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title(tr.T("config.field.gemini_key")).
				Description(tr.T("config.field.gemini_key.desc")).
				Value(&geminiKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title(tr.T("config.field.model")).
				Description(tr.T("config.field.model.desc")).
				Suggestions([]string{"gpt-4o", "claude-3-opus", "gemini-1.5-pro", "llama3", "deepseek-v4-pro"}).
				Value(&model),

			huh.NewInput().
				Title(tr.T("config.field.prompt_template")).
				Description(tr.T("config.field.prompt_template.desc")).
				Value(&promptTemplate),
		),

		huh.NewGroup(
			huh.NewInput().
				Title(tr.T("config.field.recent_n")).
				Description(tr.T("config.field.recent_n.desc")).
				Value(&recentNStr).
				Validate(func(s string) error {
					_, err := strconv.Atoi(s)
					return err
				}),

			huh.NewInput().
				Title(tr.T("config.field.max_files")).
				Description(tr.T("config.field.max_files.desc")).
				Value(&maxFilesStr).
				Validate(func(s string) error {
					_, err := strconv.Atoi(s)
					return err
				}),

			huh.NewInput().
				Title(tr.T("config.field.temperature")).
				Description(tr.T("config.field.temperature.desc")).
				Value(&tempStr).
				Validate(func(s string) error {
					v, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return err
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
					v, err := strconv.Atoi(s)
					if err != nil || v <= 0 {
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
	)

	err := form.Run()
	if err != nil {
		return cfg, false, err
	}

	cfg.APIKey = apiKey
	cfg.AnthropicKey = anthropicKey
	cfg.GeminiKey = geminiKey
	cfg.Model = model
	cfg.PromptTemplate = promptTemplate
	cfg.RulesFile = rulesFile
	cfg.Provider, cfg.BaseURL = applyOllamaOptionSelection(provider, baseURL, apiKey)

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

	return cfg, true, nil
}
