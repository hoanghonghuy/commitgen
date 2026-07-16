package app

import (
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/config"
)

// providerConfigLabel returns a human-readable provider name for config display.
func providerConfigLabel(provider, baseURL, apiKey string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		p = config.ProviderOpenAI
	}
	switch p {
	case config.ProviderOllamaCloud:
		return "ollama-cloud"
	case config.ProviderOllama:
		// Legacy combined id may still appear as "ollama" with a cloud URL/key.
		if strings.Contains(strings.ToLower(baseURL), "ollama.com") || strings.TrimSpace(apiKey) != "" {
			// Only treat as cloud when URL/key imply cloud while id is still "ollama".
			// New-schema local ollama typically has empty URL in the label call.
			if strings.Contains(strings.ToLower(baseURL), "ollama.com") {
				return "ollama-cloud"
			}
			if strings.TrimSpace(baseURL) == "" && strings.TrimSpace(apiKey) != "" {
				return "ollama-cloud"
			}
		}
		return "ollama"
	default:
		return p
	}
}
