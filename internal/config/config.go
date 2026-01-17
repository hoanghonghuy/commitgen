package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

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
}

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

func ResolveInt(flagVal int, flagSet bool, fileVal *int, defVal int) int {
	if flagSet {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defVal
}

func ResolveBool(flagVal bool, flagSet bool, fileVal *bool, defVal bool) bool {
	if flagSet {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defVal
}

func ResolveFloat(flagVal float64, flagSet bool, fileVal *float64, defVal float64) float64 {
	if flagSet {
		return flagVal
	}
	if fileVal != nil {
		return *fileVal
	}
	return defVal
}
