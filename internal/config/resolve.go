package config

import "strings"

// CredentialInput holds flag/env/file inputs used to resolve runtime credentials.
type CredentialInput struct {
	File FileConfig

	FlagProvider string
	EnvProvider  string

	FlagAPIKey string
	EnvAPIKey  string // COMMITGEN_API_KEY (active slot override)

	EnvOllamaAPIKey string // OLLAMA_API_KEY

	FlagAnthropicKey string
	EnvAnthropicKey  string
	FlagGeminiKey    string
	EnvGeminiKey     string

	FlagBaseURL string
	EnvBaseURL  string
}

// Credentials is the flat credential set consumed by app.Config / newProvider.
type Credentials struct {
	Provider     string
	BaseURL      string
	APIKey       string
	AnthropicKey string
	GeminiKey    string
}

// ResolveCredentials applies Flag > Env > File with provider registry URL rules.
func ResolveCredentials(in CredentialInput) Credentials {
	provider := ResolveString(in.FlagProvider, in.EnvProvider, in.File.Provider, ProviderOpenAI)
	provider = normalizeProviderID(provider)

	var baseURL string
	if info, ok := Lookup(provider); ok && info.FixedBaseURL != "" {
		baseURL = info.FixedBaseURL
		// Custom Ollama host preserved in CompatibleBaseURL during migrate (VB-14).
		if provider == ProviderOllama {
			if u := strings.TrimSpace(in.File.CompatibleBaseURL); u != "" {
				baseURL = u
			}
		}
	} else {
		baseURL = ResolveString(in.FlagBaseURL, in.EnvBaseURL, in.File.CompatibleBaseURL, "")
	}

	fileKey := APIKeyFor(in.File, provider)
	slot := KeySlot(provider)

	var out Credentials
	out.Provider = provider
	out.BaseURL = baseURL

	switch slot {
	case "anthropic":
		out.AnthropicKey = resolveActiveKey(in, fileKey, in.FlagAnthropicKey, in.EnvAnthropicKey, false)
	case "gemini":
		out.GeminiKey = resolveActiveKey(in, fileKey, in.FlagGeminiKey, in.EnvGeminiKey, false)
	case "ollama":
		out.APIKey = resolveOllamaKey(in, fileKey)
	default:
		out.APIKey = resolveActiveKey(in, fileKey, "", "", false)
	}
	return out
}

func resolveActiveKey(in CredentialInput, fileKey, flagSpecific, envSpecific string, _ bool) string {
	if k := strings.TrimSpace(in.FlagAPIKey); k != "" {
		return k
	}
	if k := strings.TrimSpace(flagSpecific); k != "" {
		return k
	}
	if k := strings.TrimSpace(in.EnvAPIKey); k != "" {
		return k
	}
	if k := strings.TrimSpace(envSpecific); k != "" {
		return k
	}
	return strings.TrimSpace(fileKey)
}

// resolveOllamaKey keeps official OLLAMA_API_KEY ahead of COMMITGEN_API_KEY.
func resolveOllamaKey(in CredentialInput, fileKey string) string {
	if k := strings.TrimSpace(in.FlagAPIKey); k != "" {
		return k
	}
	if k := strings.TrimSpace(in.EnvOllamaAPIKey); k != "" {
		return k
	}
	if k := strings.TrimSpace(fileKey); k != "" {
		return k
	}
	return strings.TrimSpace(in.EnvAPIKey)
}
