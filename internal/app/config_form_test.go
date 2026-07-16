package app

import (
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/config"
)

func TestStep2IncludesBaseURL(t *testing.T) {
	if step2IncludesBaseURL(config.ProviderOpenAI) {
		t.Fatal("openai must not include base URL field")
	}
	if !step2IncludesBaseURL(config.ProviderCompatible) {
		t.Fatal("compatible must include base URL field")
	}
}

func TestFileConfigFromInteractive_PreservesSiblingKeys(t *testing.T) {
	existing := config.FileConfig{
		Provider: config.ProviderOpenAI,
		APIKeys: map[string]string{
			"openai":     "oa-key",
			"openrouter": "or-key",
		},
	}
	newCfg := Config{
		Provider: config.ProviderGemini,
		Model:    "gemini-2.0-flash",
		Locale:   "en",
	}
	out := fileConfigFromInteractive(newCfg, existing, "", 5, 10, 0.7, true, true, nil)
	if out.APIKeys["openai"] != "oa-key" || out.APIKeys["openrouter"] != "or-key" {
		t.Fatalf("sibling keys dropped: %v", out.APIKeys)
	}
	if out.Provider != config.ProviderGemini {
		t.Fatalf("Provider=%q", out.Provider)
	}
}

func TestFileConfigFromInteractive_CompatibleBaseURL(t *testing.T) {
	existing := config.FileConfig{Provider: config.ProviderCompatible}
	newCfg := Config{
		Provider: config.ProviderCompatible,
		BaseURL:  "https://proxy/v1",
		Model:    "gpt-4o",
	}
	out := fileConfigFromInteractive(newCfg, existing, "k", 5, 10, 0.7, true, true, nil)
	if out.CompatibleBaseURL != "https://proxy/v1" {
		t.Fatalf("CompatibleBaseURL=%q", out.CompatibleBaseURL)
	}
	if out.APIKeys["compatible"] != "k" {
		t.Fatalf("api_keys=%v", out.APIKeys)
	}
	if out.BaseURL != "" || out.APIKey != "" {
		t.Fatalf("legacy fields should be empty")
	}
}

func TestFileConfigFromInteractive_EmptyKeyPreserves(t *testing.T) {
	existing := config.FileConfig{
		Provider: config.ProviderOpenRouter,
		APIKeys:  map[string]string{"openrouter": "keep-me"},
	}
	newCfg := Config{Provider: config.ProviderOpenRouter, Model: "m"}
	out := fileConfigFromInteractive(newCfg, existing, "", 5, 10, 0.7, true, true, nil)
	if out.APIKeys["openrouter"] != "keep-me" {
		t.Fatalf("key not preserved: %v", out.APIKeys)
	}
	out2 := fileConfigFromInteractive(newCfg, existing, secretMask, 5, 10, 0.7, true, true, nil)
	if out2.APIKeys["openrouter"] != "keep-me" {
		t.Fatalf("masked key not preserved: %v", out2.APIKeys)
	}
}
