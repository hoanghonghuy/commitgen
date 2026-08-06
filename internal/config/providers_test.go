package config

import (
	"testing"
)

func TestLookup_FixedURLs(t *testing.T) {
	tests := []struct {
		id       string
		wantURL  string
		editable bool
		keySlot  string
	}{
		{"openai", "https://api.openai.com/v1", false, "openai"},
		{"openrouter", "https://openrouter.ai/api/v1", false, "openrouter"},
		{"compatible", "", true, "compatible"},
		{"ollama", "http://localhost:11434", false, "ollama"},
		{"ollama-cloud", "https://ollama.com", false, "ollama"},
		{"anthropic", "https://api.anthropic.com/v1/messages", false, "anthropic"},
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/models", false, "gemini"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			info, ok := Lookup(tt.id)
			if !ok {
				t.Fatalf("Lookup(%q) not found", tt.id)
			}
			if info.FixedBaseURL != tt.wantURL {
				t.Errorf("FixedBaseURL = %q; want %q", info.FixedBaseURL, tt.wantURL)
			}
			if info.KeySlot != tt.keySlot {
				t.Errorf("KeySlot = %q; want %q", info.KeySlot, tt.keySlot)
			}
			if got := IsEditableBaseURL(tt.id); got != tt.editable {
				t.Errorf("IsEditableBaseURL = %v; want %v", got, tt.editable)
			}
		})
	}
}

func TestLookup_Unknown(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Fatal("expected unknown provider")
	}
}

func TestModelSuggestions_LockedTable(t *testing.T) {
	tests := map[string][]string{
		"openai":       {"gpt-4.1-mini", "gpt-4.1", "o4-mini", "gpt-4o"},
		"openrouter":   {"anthropic/claude-sonnet-4", "openai/gpt-4.1-mini", "google/gemini-2.5-flash"},
		"compatible":   {"gpt-4o", "llama3", "deepseek-v4-pro"},
		"ollama":       {"llama3", "mistral", "qwen2.5"},
		"ollama-cloud": {"deepseek-v4-pro", "llama3.3", "qwen2.5-coder"},
		"anthropic":    {"claude-sonnet-4-20250514", "claude-3-5-haiku-latest", "claude-3-opus-20240229"},
		"gemini":       {"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
	}
	for id, want := range tests {
		got := ModelSuggestions(id)
		if len(got) != len(want) {
			t.Errorf("%s: len=%d want %d (%v)", id, len(got), len(want), got)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s[%d]=%q; want %q", id, i, got[i], want[i])
			}
		}
	}
}

func TestKnownProviders(t *testing.T) {
	got := KnownProviders()
	want := []string{ProviderOpenAI, ProviderOpenRouter, ProviderCompatible, ProviderOllama, ProviderOllamaCloud, ProviderAnthropic, ProviderGemini}
	if len(got) != len(want) {
		t.Fatalf("KnownProviders len=%d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KnownProviders[%d]=%q want %q", i, got[i], want[i])
		}
	}
}
