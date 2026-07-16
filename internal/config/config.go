package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileConfig holds the application configuration loaded from a JSON file.
type FileConfig struct {
	Provider string `json:"provider,omitempty"` // openai, openrouter, compatible, ollama, ollama-cloud, anthropic, gemini
	Model    string `json:"model"`

	// New schema: per-provider API keys and optional custom URL for compatible.
	APIKeys           map[string]string `json:"api_keys,omitempty"`
	CompatibleBaseURL string            `json:"compatible_base_url,omitempty"`

	// Legacy fields — read for migration only; cleared after MigrateFileConfig.
	// omitempty so Save after migrate does not rewrite them.
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"` // legacy single OpenAI-compatible key
	AnthropicKey string `json:"anthropic_key,omitempty"`
	GeminiKey    string `json:"gemini_key,omitempty"`

	PromptTemplate string `json:"prompt_template,omitempty"`

	IgnoredFiles []string `json:"ignored_files,omitempty"`

	// Advanced Settings
	RecentN      *int     `json:"recent_n,omitempty"`
	MaxFiles     *int     `json:"max_files,omitempty"`
	Summarize    *bool    `json:"summarize,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	Conventional *bool    `json:"conventional,omitempty"`
	Timeout      *int     `json:"timeout_seconds,omitempty"` // AI request timeout in seconds

	// Prompt template loaded from a file (takes precedence over PromptTemplate when set)
	PromptTemplateFile string `json:"prompt_template_file,omitempty"`

	// Locale for UI language (en, vi, ja, zh). Defaults to "en".
	Locale string `json:"locale,omitempty"`

	// Path to commit message validation rules file (e.g. .commitgen-rules.json).
	RulesFile string `json:"rules_file,omitempty"`

	// Review Settings
	ReviewLanguage string `json:"review_language,omitempty"` // en, vi

	// Logging Settings
	LogLevel  string `json:"log_level,omitempty"`  // debug, info, warn, error
	LogOutput string `json:"log_output,omitempty"` // stdout, stderr, file, both
	LogFile   string `json:"log_file,omitempty"`   // path to log file
}

// Load reads configuration from the given path, or from ~/.commitgen.json if path is empty.
func Load(path string) (FileConfig, error) {
	var cfg FileConfig
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return cfg, nil
		}
		path = filepath.Join(home, ".commitgen.json")
	}

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	migrated, dirty := MigrateFileConfig(cfg)
	if dirty {
		if saveErr := Save(migrated, path); saveErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to rewrite migrated config %s: %v\n", path, saveErr)
		}
	}
	return migrated, nil
}

// Save writes configuration to the given path, or to ~/.commitgen.json if path is empty.
func Save(cfg FileConfig, path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, ".commitgen.json")
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0644)
}

// Merge overlays the non-empty/non-nil fields of override onto base and
// returns the result. Used to apply a repo-local config on top of the global one.
func Merge(base, override FileConfig) FileConfig {
	out := base
	if override.CompatibleBaseURL != "" {
		out.CompatibleBaseURL = override.CompatibleBaseURL
	}
	if override.BaseURL != "" {
		out.BaseURL = override.BaseURL
	}
	if override.APIKey != "" {
		out.APIKey = override.APIKey
	}
	if override.Model != "" {
		out.Model = override.Model
	}
	if override.Provider != "" {
		out.Provider = override.Provider
	}
	if override.AnthropicKey != "" {
		out.AnthropicKey = override.AnthropicKey
	}
	if override.GeminiKey != "" {
		out.GeminiKey = override.GeminiKey
	}
	out.APIKeys = mergeAPIKeys(base.APIKeys, override.APIKeys)
	if override.PromptTemplate != "" {
		out.PromptTemplate = override.PromptTemplate
	}
	if override.PromptTemplateFile != "" {
		out.PromptTemplateFile = override.PromptTemplateFile
	}
	if override.IgnoredFiles != nil {
		out.IgnoredFiles = override.IgnoredFiles
	}
	if override.RecentN != nil {
		out.RecentN = override.RecentN
	}
	if override.MaxFiles != nil {
		out.MaxFiles = override.MaxFiles
	}
	if override.Summarize != nil {
		out.Summarize = override.Summarize
	}
	if override.Temperature != nil {
		out.Temperature = override.Temperature
	}
	if override.Conventional != nil {
		out.Conventional = override.Conventional
	}
	if override.Timeout != nil {
		out.Timeout = override.Timeout
	}
	if override.ReviewLanguage != "" {
		out.ReviewLanguage = override.ReviewLanguage
	}
	if override.Locale != "" {
		out.Locale = override.Locale
	}
	if override.RulesFile != "" {
		out.RulesFile = override.RulesFile
	}
	if override.LogLevel != "" {
		out.LogLevel = override.LogLevel
	}
	if override.LogOutput != "" {
		out.LogOutput = override.LogOutput
	}
	if override.LogFile != "" {
		out.LogFile = override.LogFile
	}
	return out
}

func mergeAPIKeys(base, override map[string]string) map[string]string {
	if base == nil && override == nil {
		return nil
	}
	out := make(map[string]string)
	for k, v := range base {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	for k, v := range override {
		if strings.TrimSpace(v) != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// LoadResolved loads configuration. When explicitPath is given it loads only
// that file. Otherwise it loads the global ~/.commitgen.json and overlays a
// repo-local .commitgen.json (found by walking up from the current directory).
func LoadResolved(explicitPath string) (FileConfig, error) {
	if explicitPath != "" {
		return Load(explicitPath)
	}
	global, err := Load("")
	if err != nil {
		return global, err
	}
	if local, ok := findRepoLocalConfig(); ok {
		lc, lerr := Load(local)
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load repo config %s: %v\n", local, lerr)
			return global, nil
		}
		return Merge(global, lc), nil
	}
	return global, nil
}

// RepoLocalConfigPath walks up from the current directory looking for a
// .commitgen.json that is not the global one in the home directory.
func RepoLocalConfigPath() (string, bool) {
	return findRepoLocalConfig()
}

// findRepoLocalConfig walks up from the current directory looking for a
// .commitgen.json that is not the global one in the home directory. The search
// is bounded at the home directory so it never descends into system paths.
func findRepoLocalConfig() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	homeDir := ""
	globalPath := ""
	if home, herr := os.UserHomeDir(); herr == nil {
		homeDir = home
		globalPath = filepath.Join(home, ".commitgen.json")
	}
	cur := cwd
	for {
		p := filepath.Join(cur, ".commitgen.json")
		if p != globalPath {
			if _, statErr := os.Stat(p); statErr == nil {
				return p, true
			}
		}
		// Stop once we have inspected the home directory.
		if homeDir != "" && cur == homeDir {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", false
}

// ResolveString returns the first non-empty value from flag, env, file, or default.
func ResolveString(flagVal, envVal, fileVal, defVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if envVal != "" {
		return envVal
	}
	if fileVal != "" {
		return fileVal
	}
	return defVal
}

// ResolveInt returns the flag value if set, file value if non-nil, or default.
func ResolveInt(flagVal int, flagSet bool, fileVal *int, defVal int) int {
	if flagSet {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defVal
}

// ResolveBool returns the flag value if set, file value if non-nil, or default.
func ResolveBool(flagVal bool, flagSet bool, fileVal *bool, defVal bool) bool {
	if flagSet {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defVal
}

// ResolveFloat returns the flag value if set, file value if non-nil, or default.
func ResolveFloat(flagVal float64, flagSet bool, fileVal *float64, defVal float64) float64 {
	if flagSet {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defVal
}
