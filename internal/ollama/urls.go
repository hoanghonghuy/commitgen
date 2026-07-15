package ollama

import (
	"os"
	"strings"
)

const (
	// LocalBaseURL is the default host for a local Ollama daemon.
	LocalBaseURL = "http://localhost:11434"
	// CloudBaseURL is the official Ollama Cloud API host (see https://docs.ollama.com/api/introduction).
	CloudBaseURL = "https://ollama.com"
)

// ResolveBaseURL returns the effective Ollama host. It migrates the legacy
// incorrect api.ollama.cloud URL, defaults to Cloud when an API key is set,
// otherwise defaults to local.
func ResolveBaseURL(baseURL, apiKey string) string {
	resolved := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(resolved)
	if strings.Contains(lower, "ollama.cloud") {
		return CloudBaseURL
	}
	if resolved != "" {
		return resolved
	}
	if strings.TrimSpace(apiKey) != "" {
		return CloudBaseURL
	}
	return LocalBaseURL
}

// IsCloudBaseURL reports whether baseURL points at Ollama Cloud.
func IsCloudBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if strings.Contains(u, "ollama.com") {
		return true
	}
	return strings.Contains(u, "ollama.cloud")
}

// IsLocalBaseURL reports whether baseURL points at a local Ollama daemon.
func IsLocalBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	return strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1")
}

// ResolveAPIKey picks the Ollama API key. For the ollama provider we prefer
// OLLAMA_API_KEY (official) and the config file over COMMITGEN_API_KEY so a
// generic OpenAI/OpenRouter key in the shell does not override Ollama Cloud auth.
func ResolveAPIKey(flagVal, fileKey, commitgenEnvKey string) string {
	if k := strings.TrimSpace(flagVal); k != "" {
		return k
	}
	if k := strings.TrimSpace(os.Getenv("OLLAMA_API_KEY")); k != "" {
		return k
	}
	if k := strings.TrimSpace(fileKey); k != "" {
		return k
	}
	return strings.TrimSpace(commitgenEnvKey)
}

// DefaultModel returns a sensible default model name for local vs cloud Ollama.
func DefaultModel(baseURL, apiKey string) string {
	if IsCloudBaseURL(ResolveBaseURL(baseURL, apiKey)) {
		return "deepseek-v4-pro"
	}
	return "llama3"
}
