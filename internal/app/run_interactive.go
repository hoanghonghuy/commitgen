package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// runConfigInteractive launches a TUI form to edit key config fields
func runConfigInteractive(cfg Config) (Config, bool, error) {
	baseURL := cfg.BaseURL
	apiKey := cfg.APIKey
	anthropicKey := cfg.AnthropicKey
	geminiKey := cfg.GeminiKey
	model := cfg.Model
	provider := cfg.Provider
	if provider == "" {
		provider = "openai"
	}

	recentNStr := fmt.Sprintf("%d", cfg.RecentN)
	maxFilesStr := fmt.Sprintf("%d", cfg.MaxFiles)
	tempStr := fmt.Sprintf("%.2f", cfg.Temperature)
	summarize := cfg.Summarize
	conventional := cfg.Conventional
	ignoredFilesStr := strings.Join(cfg.IgnoredFiles, ", ")

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("CommitGen Configuration").
				Description("Update your global settings in ~/.commitgen.json"),

			huh.NewSelect[string]().
				Title("AI Provider").
				Options(
					huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Ollama (Local)", "ollama"),
					huh.NewOption("Anthropic (Claude)", "anthropic"),
					huh.NewOption("Google Gemini", "gemini"),
				).
				Value(&provider),

			huh.NewInput().
				Title("Base URL").
				Description("API endpoint (default varies by provider)").
				Placeholder("https://api.openai.com/v1 or http://localhost:11434").
				Value(&baseURL),

			huh.NewInput().
				Title("OpenAI API Key").
				Description("Key for OpenAI/Compatible providers").
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
				Suggestions([]string{"gpt-4o", "claude-3-opus", "gemini-1.5-pro", "llama3"}).
				Value(&model),
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
	)

	err := form.Run()
	if err != nil {
		return cfg, false, err
	}

	// Update the config object
	cfg.BaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.AnthropicKey = anthropicKey
	cfg.GeminiKey = geminiKey
	cfg.Model = model
	cfg.Provider = provider

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

	return cfg, true, nil
}

// Action enum for confirmation
type Action int

const (
	ActionCommit Action = iota
	ActionRegenerate
	ActionEdit
	ActionCancel
)

func confirmCommitInteractive(commitMsg string) (Action, error) {
	// Display the message nicely
	fmt.Println()
	fmt.Println(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")). // Pinkish
		Render("Generated Commit Message:"))

	fmt.Println(lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")). // Purplish
		Padding(1, 2).
		MarginBottom(1).
		Render(strings.TrimSpace(commitMsg)))

	// Since huh Select binds to a value, we need a temp var
	var selected string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do?").
				Options(
					huh.NewOption("Commit (Apply)", "commit"),
					huh.NewOption("Regenerate", "regenerate"),
					huh.NewOption("Edit", "edit"),
					huh.NewOption("Cancel", "cancel"),
				).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		return ActionCancel, err
	}

	switch selected {
	case "commit":
		return ActionCommit, nil
	case "edit":
		return ActionEdit, nil
	case "regenerate":
		return ActionRegenerate, nil
	default:
		return ActionCancel, nil
	}
}

func editCommitMessageInteractive(initialMsg string) (string, error) {
	var content string = initialMsg

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Edit Commit Message").
				Description("Modify the message below (Press Esc+Enter or standard submit key to finish)").
				Value(&content),
		),
	)

	err := form.Run()
	if err != nil {
		return "", err
	}
	return content, nil
}
