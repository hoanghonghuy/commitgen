package config

import (
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/ollama"
)

// MigrateFileConfig converts legacy provider/base_url/api_key fields into the
// registry schema (api_keys + compatible_base_url). The second return is true
// when the result should be persisted (eager rewrite).
func MigrateFileConfig(cfg FileConfig) (FileConfig, bool) {
	if !needsMigration(cfg) {
		return cfg, false
	}

	out := cfg
	if out.APIKeys == nil {
		out.APIKeys = make(map[string]string)
	} else {
		// Copy so we do not mutate caller's map.
		copied := make(map[string]string, len(out.APIKeys))
		for k, v := range out.APIKeys {
			copied[k] = v
		}
		out.APIKeys = copied
	}

	moveLegacyNamedKeys(&out)

	provider := normalizeProviderID(out.Provider)
	if provider == "" {
		provider = ProviderOpenAI
	}
	base := strings.TrimSpace(out.BaseURL)
	lowerBase := strings.ToLower(base)
	apiKey := strings.TrimSpace(out.APIKey)

	switch provider {
	case ProviderOpenAI:
		switch {
		case strings.Contains(lowerBase, "openrouter.ai"):
			out.Provider = ProviderOpenRouter
			setKeyIfEmpty(out.APIKeys, "openrouter", apiKey)
		case base == "" || strings.Contains(lowerBase, "api.openai.com"):
			out.Provider = ProviderOpenAI
			setKeyIfEmpty(out.APIKeys, "openai", apiKey)
		default:
			out.Provider = ProviderCompatible
			if out.CompatibleBaseURL == "" {
				out.CompatibleBaseURL = base
			}
			setKeyIfEmpty(out.APIKeys, "compatible", apiKey)
		}
	case ProviderOllama:
		resolved := ollama.ResolveBaseURL(base, apiKey)
		if ollama.IsCloudBaseURL(resolved) {
			out.Provider = ProviderOllamaCloud
		} else {
			out.Provider = ProviderOllama
		}
		setKeyIfEmpty(out.APIKeys, "ollama", apiKey)
	case ProviderOpenRouter:
		setKeyIfEmpty(out.APIKeys, "openrouter", apiKey)
		out.Provider = ProviderOpenRouter
	case ProviderCompatible:
		out.Provider = ProviderCompatible
		if out.CompatibleBaseURL == "" && base != "" {
			out.CompatibleBaseURL = base
		}
		setKeyIfEmpty(out.APIKeys, "compatible", apiKey)
	case ProviderOllamaCloud:
		out.Provider = ProviderOllamaCloud
		setKeyIfEmpty(out.APIKeys, "ollama", apiKey)
	case ProviderAnthropic:
		out.Provider = ProviderAnthropic
		setKeyIfEmpty(out.APIKeys, "anthropic", apiKey)
	case ProviderGemini:
		out.Provider = ProviderGemini
		setKeyIfEmpty(out.APIKeys, "gemini", apiKey)
	default:
		out.Provider = provider
		if apiKey != "" {
			setKeyIfEmpty(out.APIKeys, KeySlot(provider), apiKey)
		}
	}

	out.APIKey = ""
	out.BaseURL = ""
	out.AnthropicKey = ""
	out.GeminiKey = ""

	if len(out.APIKeys) == 0 {
		out.APIKeys = nil
	}
	return out, true
}

func needsMigration(cfg FileConfig) bool {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		return true
	}
	if strings.TrimSpace(cfg.AnthropicKey) != "" {
		return true
	}
	if strings.TrimSpace(cfg.GeminiKey) != "" {
		return true
	}
	return false
}

func moveLegacyNamedKeys(cfg *FileConfig) {
	if k := strings.TrimSpace(cfg.AnthropicKey); k != "" {
		setKeyIfEmpty(cfg.APIKeys, "anthropic", k)
	}
	if k := strings.TrimSpace(cfg.GeminiKey); k != "" {
		setKeyIfEmpty(cfg.APIKeys, "gemini", k)
	}
}

func setKeyIfEmpty(m map[string]string, slot, value string) {
	if m == nil || strings.TrimSpace(value) == "" {
		return
	}
	if strings.TrimSpace(m[slot]) == "" {
		m[slot] = value
	}
}

// APIKeyFor returns the stored key for a provider's key slot.
func APIKeyFor(cfg FileConfig, provider string) string {
	slot := KeySlot(provider)
	if cfg.APIKeys != nil {
		if k := strings.TrimSpace(cfg.APIKeys[slot]); k != "" {
			return k
		}
	}
	// Legacy fallback before migrate.
	switch slot {
	case "anthropic":
		if k := strings.TrimSpace(cfg.AnthropicKey); k != "" {
			return k
		}
	case "gemini":
		if k := strings.TrimSpace(cfg.GeminiKey); k != "" {
			return k
		}
	}
	return strings.TrimSpace(cfg.APIKey)
}

// SetAPIKeyFor sets a key in api_keys for the provider slot.
func SetAPIKeyFor(cfg *FileConfig, provider, key string) {
	if cfg.APIKeys == nil {
		cfg.APIKeys = make(map[string]string)
	}
	slot := KeySlot(provider)
	cfg.APIKeys[slot] = key
}

// PreserveAPIKeyIfEmpty keeps the existing slot key when submitted is empty or masked.
func PreserveAPIKeyIfEmpty(cfg *FileConfig, provider, submitted, existing string) {
	s := strings.TrimSpace(submitted)
	if s == "" || s == "********" {
		if strings.TrimSpace(existing) != "" {
			SetAPIKeyFor(cfg, provider, existing)
		}
		return
	}
	SetAPIKeyFor(cfg, provider, s)
}
