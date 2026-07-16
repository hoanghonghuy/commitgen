package config

import "testing"

func TestResolveCredentials(t *testing.T) {
	file := FileConfig{
		Provider:          ProviderOpenRouter,
		APIKeys:           map[string]string{"openrouter": "file-or", "openai": "file-oa", "ollama": "file-ol"},
		CompatibleBaseURL: "https://proxy.example/v1",
	}

	t.Run("openrouter uses fixed url and file key", func(t *testing.T) {
		got := ResolveCredentials(CredentialInput{File: file})
		if got.Provider != ProviderOpenRouter {
			t.Fatalf("Provider=%q", got.Provider)
		}
		if got.BaseURL != "https://openrouter.ai/api/v1" {
			t.Fatalf("BaseURL=%q", got.BaseURL)
		}
		if got.APIKey != "file-or" {
			t.Fatalf("APIKey=%q", got.APIKey)
		}
	})

	t.Run("commitgen env overrides active key", func(t *testing.T) {
		got := ResolveCredentials(CredentialInput{
			File:      file,
			EnvAPIKey: "env-or",
		})
		if got.APIKey != "env-or" {
			t.Fatalf("APIKey=%q", got.APIKey)
		}
	})

	t.Run("base url flag ignored for openai", func(t *testing.T) {
		got := ResolveCredentials(CredentialInput{
			File:        FileConfig{Provider: ProviderOpenAI, APIKeys: map[string]string{"openai": "k"}},
			FlagBaseURL: "https://evil.example/v1",
			EnvBaseURL:  "https://evil-env.example/v1",
		})
		if got.BaseURL != "https://api.openai.com/v1" {
			t.Fatalf("BaseURL=%q", got.BaseURL)
		}
	})

	t.Run("compatible uses flag base url", func(t *testing.T) {
		got := ResolveCredentials(CredentialInput{
			File: FileConfig{
				Provider:          ProviderCompatible,
				CompatibleBaseURL: "https://file-proxy/v1",
				APIKeys:           map[string]string{"compatible": "k"},
			},
			FlagBaseURL: "https://flag-proxy/v1",
		})
		if got.BaseURL != "https://flag-proxy/v1" {
			t.Fatalf("BaseURL=%q", got.BaseURL)
		}
	})

	t.Run("ollama cloud fixed url and ollama env key", func(t *testing.T) {
		got := ResolveCredentials(CredentialInput{
			File:            FileConfig{Provider: ProviderOllamaCloud, APIKeys: map[string]string{"ollama": "file-ol"}},
			EnvOllamaAPIKey: "env-ol",
			EnvAPIKey:       "commitgen-should-lose",
		})
		if got.BaseURL != "https://ollama.com" {
			t.Fatalf("BaseURL=%q", got.BaseURL)
		}
		if got.APIKey != "env-ol" {
			t.Fatalf("APIKey=%q; want OLLAMA env over COMMITGEN", got.APIKey)
		}
	})

	t.Run("custom ollama host from compatible_base_url", func(t *testing.T) {
		got := ResolveCredentials(CredentialInput{
			File: FileConfig{
				Provider:          ProviderOllama,
				CompatibleBaseURL: "http://192.168.1.10:11434",
			},
		})
		if got.BaseURL != "http://192.168.1.10:11434" {
			t.Fatalf("BaseURL=%q", got.BaseURL)
		}
	})
}
