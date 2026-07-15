package app

import "testing"

func TestOllamaOptionForConfig(t *testing.T) {
	tests := []struct {
		provider, baseURL, apiKey, want string
	}{
		{"openai", "", "", "openai"},
		{"ollama", "", "", ollamaLocalOption},
		{"ollama", "http://localhost:11434", "", ollamaLocalOption},
		{"ollama", "http://localhost:11434", "sk-key", ollamaLocalOption},
		{"ollama", "https://ollama.com", "", ollamaCloudOption},
		{"ollama", "https://api.ollama.cloud", "", ollamaCloudOption},
		{"ollama", "", "sk-cloud", ollamaCloudOption},
	}
	for _, tt := range tests {
		if got := ollamaOptionForConfig(tt.provider, tt.baseURL, tt.apiKey); got != tt.want {
			t.Errorf("ollamaOptionForConfig(%q,%q,%q) = %q, want %q", tt.provider, tt.baseURL, tt.apiKey, got, tt.want)
		}
	}
}

func TestApplyOllamaOptionSelection(t *testing.T) {
	p, u := applyOllamaOptionSelection(ollamaLocalOption, "", "")
	if p != "ollama" || u != "http://localhost:11434" {
		t.Fatalf("local defaults: provider=%q baseURL=%q", p, u)
	}
	p, u = applyOllamaOptionSelection(ollamaCloudOption, "", "")
	if p != "ollama" || u != "https://ollama.com" {
		t.Fatalf("cloud defaults: provider=%q baseURL=%q", p, u)
	}
	p, u = applyOllamaOptionSelection(ollamaCloudOption, "https://api.ollama.cloud", "")
	if p != "ollama" || u != "https://ollama.com" {
		t.Fatalf("legacy api.ollama.cloud should migrate to ollama.com, got %q", u)
	}
	p, u = applyOllamaOptionSelection("openai", "https://api.openai.com/v1", "")
	if p != "openai" || u != "https://api.openai.com/v1" {
		t.Fatalf("openai passthrough: provider=%q baseURL=%q", p, u)
	}
}
