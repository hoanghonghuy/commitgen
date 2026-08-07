package ollama

import "testing"

func TestResolveBaseURL(t *testing.T) {
	tests := []struct {
		baseURL, apiKey, want string
	}{
		{"", "", LocalBaseURL},
		{"", "sk-key", CloudBaseURL},
		{"http://localhost:11434", "sk-key", "http://localhost:11434"},
		{"https://ollama.com", "", CloudBaseURL},
		{"https://api.ollama.cloud", "", CloudBaseURL},
		{"http://192.168.1.5:11434", "", "http://192.168.1.5:11434"},
	}
	for _, tt := range tests {
		if got := ResolveBaseURL(tt.baseURL, tt.apiKey); got != tt.want {
			t.Errorf("ResolveBaseURL(%q,%q) = %q, want %q", tt.baseURL, tt.apiKey, got, tt.want)
		}
	}
}

func TestIsCloudBaseURL(t *testing.T) {
	if !IsCloudBaseURL("https://ollama.com") {
		t.Error("ollama.com should be cloud")
	}
	if !IsCloudBaseURL("https://api.ollama.cloud") {
		t.Error("legacy ollama.cloud should be cloud")
	}
	if IsCloudBaseURL("http://localhost:11434") {
		t.Error("localhost should not be cloud")
	}
}

func TestIsLocalBaseURL(t *testing.T) {
	if !IsLocalBaseURL("http://localhost:11434/") {
		t.Error("localhost should be local")
	}
	if !IsLocalBaseURL("http://127.0.0.1:11434") {
		t.Error("127.0.0.1 should be local")
	}
	if IsLocalBaseURL("https://ollama.com") {
		t.Error("ollama.com should not be local")
	}
}

func TestResolveAPIKey(t *testing.T) {
	t.Setenv("OLLAMA_API_KEY", "")
	if got := ResolveAPIKey("flag-key", "file-key", "commitgen-env"); got != "flag-key" {
		t.Fatalf("flag = %q, want flag-key", got)
	}
	t.Setenv("OLLAMA_API_KEY", "ollama-env")
	if got := ResolveAPIKey("", "file-key", "commitgen-env"); got != "ollama-env" {
		t.Fatalf("OLLAMA_API_KEY = %q, want ollama-env", got)
	}
	t.Setenv("OLLAMA_API_KEY", "")
	if got := ResolveAPIKey("", "file-key", "commitgen-env"); got != "file-key" {
		t.Fatalf("file key = %q, want file-key", got)
	}
	if got := ResolveAPIKey("", "", "commitgen-env"); got != "commitgen-env" {
		t.Fatalf("commitgen env = %q, want commitgen-env", got)
	}
}

func TestDefaultModel(t *testing.T) {
	if got := DefaultModel("", ""); got != "llama3" {
		t.Fatalf("local default = %q", got)
	}
	if got := DefaultModel("https://ollama.com", "sk-key"); got != "deepseek-v4-pro" {
		t.Fatalf("cloud default = %q", got)
	}
}
