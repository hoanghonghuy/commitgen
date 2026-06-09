package ai

import (
	"context"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// Provider defines the interface for an AI backend (e.g. OpenAI, Ollama, Anthropic)
type Provider interface {
	Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error)
}

// StreamProvider is an optional interface for providers that can stream the
// response incrementally. onDelta is called for each text chunk as it arrives;
// the full accumulated text is returned at the end. Providers that do not
// implement this interface fall back to the blocking Generate call.
type StreamProvider interface {
	GenerateStream(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64, onDelta func(string)) (string, error)
}
