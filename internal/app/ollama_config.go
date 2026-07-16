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
		// Only treat legacy combined id as cloud when URL clearly points at cloud.
		if strings.Contains(strings.ToLower(baseURL), "ollama.com") {
			return "ollama-cloud"
		}
		return "ollama"
	default:
		return p
	}
}
