package config

import "strings"

// Provider identifiers (stable config / CLI values).
const (
	ProviderOpenAI      = "openai"
	ProviderOpenRouter  = "openrouter"
	ProviderCompatible  = "compatible"
	ProviderOllama      = "ollama"
	ProviderOllamaCloud = "ollama-cloud"
	ProviderAnthropic   = "anthropic"
	ProviderGemini      = "gemini"
)

// ClientKind identifies which HTTP client package handles a provider.
type ClientKind string

const (
	ClientOpenAI    ClientKind = "openai"
	ClientOllama    ClientKind = "ollama"
	ClientAnthropic ClientKind = "anthropic"
	ClientGemini    ClientKind = "gemini"
)

// ProviderInfo describes a registered AI backend.
type ProviderInfo struct {
	ID           string
	FixedBaseURL string // empty means editable (compatible only)
	KeySlot      string
	Kind         ClientKind
}

var providerRegistry = map[string]ProviderInfo{
	ProviderOpenAI: {
		ID:           ProviderOpenAI,
		FixedBaseURL: "https://api.openai.com/v1",
		KeySlot:      "openai",
		Kind:         ClientOpenAI,
	},
	ProviderOpenRouter: {
		ID:           ProviderOpenRouter,
		FixedBaseURL: "https://openrouter.ai/api/v1",
		KeySlot:      "openrouter",
		Kind:         ClientOpenAI,
	},
	ProviderCompatible: {
		ID:           ProviderCompatible,
		FixedBaseURL: "",
		KeySlot:      "compatible",
		Kind:         ClientOpenAI,
	},
	ProviderOllama: {
		ID:           ProviderOllama,
		FixedBaseURL: "http://localhost:11434",
		KeySlot:      "ollama",
		Kind:         ClientOllama,
	},
	ProviderOllamaCloud: {
		ID:           ProviderOllamaCloud,
		FixedBaseURL: "https://ollama.com",
		KeySlot:      "ollama",
		Kind:         ClientOllama,
	},
	ProviderAnthropic: {
		ID:           ProviderAnthropic,
		FixedBaseURL: "https://api.anthropic.com/v1/messages",
		KeySlot:      "anthropic",
		Kind:         ClientAnthropic,
	},
	ProviderGemini: {
		ID:           ProviderGemini,
		FixedBaseURL: "https://generativelanguage.googleapis.com/v1beta/models",
		KeySlot:      "gemini",
		Kind:         ClientGemini,
	},
}

var modelSuggestions = map[string][]string{
	ProviderOpenAI:      {"gpt-4.1-mini", "gpt-4.1", "o4-mini", "gpt-4o"},
	ProviderOpenRouter:  {"anthropic/claude-sonnet-4", "openai/gpt-4.1-mini", "google/gemini-2.5-flash"},
	ProviderCompatible:  {"gpt-4o", "llama3", "deepseek-v4-pro"},
	ProviderOllama:      {"llama3", "mistral", "qwen2.5"},
	ProviderOllamaCloud: {"deepseek-v4-pro", "llama3.3", "qwen2.5-coder"},
	ProviderAnthropic:   {"claude-sonnet-4-20250514", "claude-3-5-haiku-latest", "claude-3-opus-20240229"},
	ProviderGemini:      {"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
}

// Lookup returns registry metadata for a provider id.
func Lookup(id string) (ProviderInfo, bool) {
	info, ok := providerRegistry[normalizeProviderID(id)]
	return info, ok
}

// IsEditableBaseURL reports whether the provider allows a custom base URL.
func IsEditableBaseURL(id string) bool {
	info, ok := Lookup(id)
	if !ok {
		return false
	}
	return info.FixedBaseURL == ""
}

// KeySlot returns the api_keys map key for the provider.
func KeySlot(id string) string {
	info, ok := Lookup(id)
	if !ok {
		return normalizeProviderID(id)
	}
	return info.KeySlot
}

// ModelSuggestions returns the locked static suggestion list for a provider.
func ModelSuggestions(id string) []string {
	if s, ok := modelSuggestions[normalizeProviderID(id)]; ok {
		out := make([]string, len(s))
		copy(out, s)
		return out
	}
	return nil
}

// KnownProviders returns provider ids in a stable display order.
func KnownProviders() []string {
	return []string{
		ProviderOpenAI,
		ProviderOpenRouter,
		ProviderCompatible,
		ProviderOllama,
		ProviderOllamaCloud,
		ProviderAnthropic,
		ProviderGemini,
	}
}

func normalizeProviderID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
