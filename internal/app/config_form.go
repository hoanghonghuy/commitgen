package app

import (
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/config"
)

func step2IncludesBaseURL(provider string) bool {
	return config.IsEditableBaseURL(provider)
}

// fileConfigFromInteractive builds the new-schema FileConfig from form results,
// preserving sibling api_keys from existing.
func fileConfigFromInteractive(
	newCfg Config,
	existing config.FileConfig,
	submittedKey string,
	recentN, maxFiles int,
	temperature float64,
	summarize, conventional bool,
	timeoutSeconds *int,
) config.FileConfig {
	out := existing
	if out.APIKeys != nil {
		copied := make(map[string]string, len(out.APIKeys))
		for k, v := range out.APIKeys {
			copied[k] = v
		}
		out.APIKeys = copied
	}

	out.Provider = strings.TrimSpace(newCfg.Provider)
	if out.Provider == "" {
		out.Provider = config.ProviderOpenAI
	}
	out.Model = newCfg.Model
	out.IgnoredFiles = newCfg.IgnoredFiles
	out.PromptTemplate = newCfg.PromptTemplate
	out.ReviewLanguage = newCfg.ReviewLanguage
	out.Locale = newCfg.Locale
	out.RulesFile = newCfg.RulesFile
	out.LogLevel = newCfg.LogLevel
	out.LogOutput = newCfg.LogOutput
	out.LogFile = newCfg.LogFile
	out.PromptTemplateFile = firstNonEmpty(newCfg.PromptTemplateFile, existing.PromptTemplateFile)

	out.RecentN = &recentN
	out.MaxFiles = &maxFiles
	out.Temperature = &temperature
	out.Summarize = &summarize
	out.Conventional = &conventional
	if timeoutSeconds != nil {
		out.Timeout = timeoutSeconds
	} else if existing.Timeout != nil {
		out.Timeout = existing.Timeout
	}

	existingKey := config.APIKeyFor(existing, out.Provider)
	config.PreserveAPIKeyIfEmpty(&out, out.Provider, submittedKey, existingKey)

	if step2IncludesBaseURL(out.Provider) {
		out.CompatibleBaseURL = strings.TrimSpace(newCfg.BaseURL)
	}

	out.APIKey = ""
	out.BaseURL = ""
	out.AnthropicKey = ""
	out.GeminiKey = ""
	return out
}
