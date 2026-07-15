package app

import (
	"fmt"
	"strconv"
	"strings"

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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("CommitGen Configuration").
				Description(tr.T("config.form_intro") + "\n" + tr.T("config.save_target", savePath)),

			huh.NewSelect[string]().
				Title("AI Provider").
				Options(
					huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Ollama (Local)", ollamaLocalOption),
					huh.NewOption("Ollama (Cloud)", ollamaCloudOption),
					huh.NewOption("Anthropic (Claude)", "anthropic"),
					huh.NewOption("Google Gemini", "gemini"),
				).
				Value(&provider),

			huh.NewInput().
				Title("Base URL").
				Description("API endpoint — Ollama Local: localhost:11434; Ollama Cloud: ollama.com; or OpenAI-compatible URL").
				Placeholder("https://api.openai.com/v1 | http://localhost:11434 | https://ollama.com").
				Suggestions([]string{
					"https://api.openai.com/v1",
					"https://openrouter.ai/api/v1",
					"https://api.mistral.ai/v1",
					"https://ollama.com",
					"http://localhost:11434",
				}).
				Value(&baseURL),

			huh.NewInput().
				Title("API Key").
				Description("Key for OpenAI / Ollama Cloud / Compatible providers").
				Value(&apiKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title("Anthropic API Key").
				Description("Key for Claude models").
				Value(&anthropicKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title("Gemini API Key").
				Description("Key for Google Gemini").
				Value(&geminiKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title("Model").
				Description("Model name").
				Suggestions([]string{"gpt-4o", "claude-3-opus", "gemini-1.5-pro", "llama3", "deepseek-v4-pro"}).
				Value(&model),

			huh.NewInput().
				Title("System Prompt Template").
				Description("Custom system prompt (leave empty for default)").
				Value(&promptTemplate),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Recent Commits").
				Description("Number of recent commits to include").
				Value(&recentNStr).
				Validate(func(s string) error {
					_, err := strconv.Atoi(s)
					return err
				}),

			huh.NewInput().
				Title("Max Files").
				Description("Max staged files to verify").
				Value(&maxFilesStr).
				Validate(func(s string) error {
					_, err := strconv.Atoi(s)
					return err
				}),

			huh.NewInput().
				Title("Temperature").
				Description("LLM Temperature (0.0 - 2.0)").
				Value(&tempStr).
				Validate(func(s string) error {
					v, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return err
					}
					if v < 0 || v > 2.0 {
						return fmt.Errorf("must be between 0.0 and 2.0")
					}
					return nil
				}),
		),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Summarize Changes").
				Description("Summarize file content for larger files?").
				Value(&summarize),

			huh.NewConfirm().
				Title("Conventional Commits").
				Description("Enforce Conventional Commits specification?").
				Value(&conventional),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Ignored Files").
				Description("Glob patterns (comma separated)").
				Value(&ignoredFilesStr),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("UI Language").
				Description("Language for the terminal interface").
				Options(
					huh.NewOption("English", "en"),
					huh.NewOption("Tiếng Việt", "vi"),
					huh.NewOption("日本語", "ja"),
					huh.NewOption("中文", "zh"),
				).
				Value(&locale),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Review Language").
				Description("Language for review output").
				Options(
					huh.NewOption("English", "en"),
					huh.NewOption("Tiếng Việt", "vi"),
				).
				Value(&reviewLanguage),
		),

		huh.NewGroup(
			huh.NewNote().
				Title("Logging Settings").
				Description("Configure logging behavior"),

			huh.NewSelect[string]().
				Title("Log Level").
				Options(
					huh.NewOption("Debug", "debug"),
					huh.NewOption("Info", "info"),
					huh.NewOption("Warning", "warn"),
					huh.NewOption("Error", "error"),
				).
				Value(&logLevel),

			huh.NewSelect[string]().
				Title("Log Output").
				Options(
					huh.NewOption("Console (stderr)", "stderr"),
					huh.NewOption("File only", "file"),
					huh.NewOption("Both console and file", "both"),
				).
				Value(&logOutput),

			huh.NewInput().
				Title("Log File Path").
				Description("Leave empty for default (~/.commitgen/commitgen.log)").
				Value(&logFile),
		),
	)

	err := form.Run()
	if err != nil {
		return cfg, false, err
	}

	// Update the config object
	cfg.APIKey = apiKey
	cfg.AnthropicKey = anthropicKey
	cfg.GeminiKey = geminiKey
	cfg.Model = model
	cfg.PromptTemplate = promptTemplate
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
	cfg.Summarize = summarize
	cfg.Conventional = conventional

	// Split ignored files
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
