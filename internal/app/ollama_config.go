package app

import (
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/ollama"
)

const (
	ollamaLocalOption = "ollama-local"
	ollamaCloudOption = "ollama-cloud"
)

// ollamaOptionForConfig maps stored provider/base_url/api_key to the config form
// provider select value (ollama-local vs ollama-cloud).
func ollamaOptionForConfig(provider, baseURL, apiKey string) string {
	if strings.ToLower(strings.TrimSpace(provider)) != "ollama" {
		return provider
	}
	resolved := strings.TrimSpace(baseURL)
	if resolved != "" {
		if ollama.IsLocalBaseURL(resolved) {
			return ollamaLocalOption
		}
		if ollama.IsCloudBaseURL(resolved) {
			return ollamaCloudOption
		}
	}
	if strings.TrimSpace(apiKey) != "" {
		return ollamaCloudOption
	}
	return ollamaLocalOption
}

// applyOllamaOptionSelection converts a form provider choice back to the stored
// provider value and applies sensible default base URLs for each Ollama mode.
func applyOllamaOptionSelection(option, baseURL, apiKey string) (provider, resolvedBaseURL string) {
	switch option {
	case ollamaLocalOption:
		resolved := strings.TrimSpace(baseURL)
		if resolved == "" || ollama.IsCloudBaseURL(resolved) {
			return "ollama", ollama.LocalBaseURL
		}
		return "ollama", resolved
	case ollamaCloudOption:
		resolved := strings.TrimSpace(baseURL)
		if resolved == "" || ollama.IsLocalBaseURL(resolved) {
			return "ollama", ollama.CloudBaseURL
		}
		return "ollama", ollama.ResolveBaseURL(resolved, apiKey)
	default:
		return option, baseURL
	}
}
