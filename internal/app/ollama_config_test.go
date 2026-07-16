package app

import (
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/config"
)

func TestProviderConfigLabel(t *testing.T) {
	if got := providerConfigLabel(config.ProviderOllamaCloud, "", "sk"); got != "ollama-cloud" {
		t.Fatalf("got %q", got)
	}
	if got := providerConfigLabel(config.ProviderOllama, "", ""); got != "ollama" {
		t.Fatalf("got %q", got)
	}
	if got := providerConfigLabel("ollama", "https://ollama.com", "sk"); got != "ollama-cloud" {
		t.Fatalf("legacy cloud label: %q", got)
	}
	if got := providerConfigLabel("ollama", "http://localhost:11434", ""); got != "ollama" {
		t.Fatalf("legacy local label: %q", got)
	}
	if got := providerConfigLabel(config.ProviderOllama, "", "sk-local"); got != "ollama" {
		t.Fatalf("local with key must stay ollama, got %q", got)
	}
	if got := providerConfigLabel(config.ProviderOpenAI, "", ""); got != "openai" {
		t.Fatalf("got %q", got)
	}
	if got := providerConfigLabel(config.ProviderOpenRouter, "", ""); got != "openrouter" {
		t.Fatalf("got %q", got)
	}
}
