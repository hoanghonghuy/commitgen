package ai

import (
	"context"

	"commitgen/internal/vscodeprompt"
)

// Provider defines the interface for an AI backend (e.g. OpenAI, Ollama, Anthropic)
type Provider interface {
	// GenerateCommitMessage sends the prompt to the AI and returns the generated commit message text.
	GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error)
}
