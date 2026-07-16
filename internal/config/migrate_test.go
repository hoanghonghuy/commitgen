package config

import (
	"testing"
)

func TestMigrateFileConfig_Table(t *testing.T) {
	tests := []struct {
		name         string
		in           FileConfig
		wantProvider string
		wantSlot     string
		wantKey      string
		wantCompat   string
		wantDirty    bool
	}{
		{
			name: "openrouter shaped openai",
			in: FileConfig{
				Provider: "openai",
				BaseURL:  "https://openrouter.ai/api/v1",
				APIKey:   "or-key",
			},
			wantProvider: ProviderOpenRouter,
			wantSlot:     "openrouter",
			wantKey:      "or-key",
			wantDirty:    true,
		},
		{
			name: "official openai empty url",
			in: FileConfig{
				Provider: "openai",
				APIKey:   "oa-key",
			},
			wantProvider: ProviderOpenAI,
			wantSlot:     "openai",
			wantKey:      "oa-key",
			wantDirty:    true,
		},
		{
			name: "openai.com url",
			in: FileConfig{
				Provider: "openai",
				BaseURL:  "https://api.openai.com/v1",
				APIKey:   "oa-key",
			},
			wantProvider: ProviderOpenAI,
			wantSlot:     "openai",
			wantKey:      "oa-key",
			wantDirty:    true,
		},
		{
			name: "custom proxy becomes compatible",
			in: FileConfig{
				Provider: "openai",
				BaseURL:  "https://my-proxy.example/v1",
				APIKey:   "proxy-key",
			},
			wantProvider: ProviderCompatible,
			wantSlot:     "compatible",
			wantKey:      "proxy-key",
			wantCompat:   "https://my-proxy.example/v1",
			wantDirty:    true,
		},
		{
			name: "ollama cloud url",
			in: FileConfig{
				Provider: "ollama",
				BaseURL:  "https://ollama.com",
				APIKey:   "ol-key",
			},
			wantProvider: ProviderOllamaCloud,
			wantSlot:     "ollama",
			wantKey:      "ol-key",
			wantDirty:    true,
		},
		{
			name: "ollama local",
			in: FileConfig{
				Provider: "ollama",
				BaseURL:  "http://localhost:11434",
			},
			wantProvider: ProviderOllama,
			wantDirty:    true,
		},
		{
			name: "anthropic and gemini legacy keys",
			in: FileConfig{
				Provider:     "anthropic",
				AnthropicKey: "ant-key",
				GeminiKey:    "gem-key",
			},
			wantProvider: ProviderAnthropic,
			wantSlot:     "anthropic",
			wantKey:      "ant-key",
			wantDirty:    true,
		},
		{
			name: "ollama custom lan url preserved",
			in: FileConfig{
				Provider: "ollama",
				BaseURL:  "http://192.168.1.10:11434",
				APIKey:   "",
			},
			wantProvider: ProviderOllama,
			wantCompat:   "http://192.168.1.10:11434",
			wantDirty:    true,
		},
		{
			name: "already migrated",
			in: FileConfig{
				Provider: ProviderOpenRouter,
				APIKeys:  map[string]string{"openrouter": "keep"},
			},
			wantProvider: ProviderOpenRouter,
			wantSlot:     "openrouter",
			wantKey:      "keep",
			wantDirty:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, dirty := MigrateFileConfig(tt.in)
			if dirty != tt.wantDirty {
				t.Errorf("dirty=%v; want %v", dirty, tt.wantDirty)
			}
			if out.Provider != tt.wantProvider {
				t.Errorf("Provider=%q; want %q", out.Provider, tt.wantProvider)
			}
			if tt.wantSlot != "" && out.APIKeys[tt.wantSlot] != tt.wantKey {
				t.Errorf("APIKeys[%s]=%q; want %q (map=%v)", tt.wantSlot, out.APIKeys[tt.wantSlot], tt.wantKey, out.APIKeys)
			}
			if tt.wantCompat != "" && out.CompatibleBaseURL != tt.wantCompat {
				t.Errorf("CompatibleBaseURL=%q; want %q", out.CompatibleBaseURL, tt.wantCompat)
			}
			if out.APIKey != "" || out.BaseURL != "" || out.AnthropicKey != "" || out.GeminiKey != "" {
				t.Errorf("legacy fields should be cleared, got api_key=%q base_url=%q anthropic=%q gemini=%q",
					out.APIKey, out.BaseURL, out.AnthropicKey, out.GeminiKey)
			}
			if tt.name == "anthropic and gemini legacy keys" {
				if out.APIKeys["gemini"] != "gem-key" {
					t.Errorf("APIKeys[gemini]=%q", out.APIKeys["gemini"])
				}
			}
		})
	}
}
