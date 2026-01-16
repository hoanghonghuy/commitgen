package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// runConfigInteractive launches a TUI form to edit key config fields
func runConfigInteractive(cfg Config) (Config, bool, error) {
	// We operate on a copy or just locals?
	// The fields we want to edit: BaseURL, APIKey, Model.
	// We also might want to edit RepoArg? Maybe not for global config command.

	baseURL := cfg.BaseURL
	apiKey := cfg.APIKey
	model := cfg.Model

	// Theme customization: Draculi-ish or default? Default is usually fine to start.

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("CommitGen Configuration").
				Description("Update your global settings in ~/.commitgen.json"),

			huh.NewInput().
				Title("Base URL").
				Description("OpenAI-compatible endpoint").
				Placeholder("https://api.openai.com/v1").
				Value(&baseURL),

			huh.NewInput().
				Title("API Key").
				Description("Your secret key").
				Value(&apiKey).
				EchoMode(huh.EchoModePassword),

			huh.NewInput().
				Title("Model").
				Description("Model name (e.g. gpt-4, gpt-3.5-turbo)").
				Suggestions([]string{"gpt-4o", "gpt-4-turbo", "gpt-3.5-turbo", "claude-3-opus", "claude-3-sonnet"}).
				Value(&model),
		),
	)

	err := form.Run()
	if err != nil {
		return cfg, false, err
	}

	// Update the config object
	cfg.BaseURL = baseURL
	cfg.APIKey = apiKey
	cfg.Model = model

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
