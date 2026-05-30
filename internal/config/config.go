package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FileConfig holds the application configuration loaded from a JSON file.
type FileConfig struct {
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"` // OpenAI Key
	Model    string `json:"model"`
	Provider string `json:"provider,omitempty"` // openai, ollama, anthropic, gemini

	// Provider specifics
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
	return cfg, nil
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
