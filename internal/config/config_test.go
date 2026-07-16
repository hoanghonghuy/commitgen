package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveString(t *testing.T) {
	tests := []struct {
		name                           string
		flagVal, envVal, fileVal, def  string
		want                           string
	}{
		{"flag wins", "flag", "env", "file", "def", "flag"},
		{"env when no flag", "", "env", "file", "def", "env"},
		{"file when no flag/env", "", "", "file", "def", "file"},
		{"default when all empty", "", "", "", "def", "def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveString(tt.flagVal, tt.envVal, tt.fileVal, tt.def); got != tt.want {
				t.Errorf("ResolveString = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestResolveInt(t *testing.T) {
	five := 5
	tests := []struct {
		name    string
		flagVal int
		flagSet bool
		fileVal *int
		def     int
		want    int
	}{
		{"flag set wins", 9, true, &five, 1, 9},
		{"flag set zero respected", 0, true, &five, 1, 0},
		{"file when flag not set", 9, false, &five, 1, 5},
		{"default when file nil", 9, false, nil, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveInt(tt.flagVal, tt.flagSet, tt.fileVal, tt.def); got != tt.want {
				t.Errorf("ResolveInt = %d; want %d", got, tt.want)
			}
		})
	}
}

func TestResolveBool(t *testing.T) {
	tru := true
	fal := false
	tests := []struct {
		name    string
		flagVal bool
		flagSet bool
		fileVal *bool
		def     bool
		want    bool
	}{
		{"flag set wins", true, true, &fal, false, true},
		{"flag set false respected", false, true, &tru, true, false},
		{"file when flag not set", true, false, &fal, true, false},
		{"default when file nil", true, false, nil, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveBool(tt.flagVal, tt.flagSet, tt.fileVal, tt.def); got != tt.want {
				t.Errorf("ResolveBool = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestResolveFloat(t *testing.T) {
	half := 0.5
	tests := []struct {
		name    string
		flagVal float64
		flagSet bool
		fileVal *float64
		def     float64
		want    float64
	}{
		{"flag set wins", 1.5, true, &half, 0.1, 1.5},
		{"flag set zero respected", 0, true, &half, 0.1, 0},
		{"file when flag not set", 1.5, false, &half, 0.1, 0.5},
		{"default when file nil", 1.5, false, nil, 0.1, 0.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveFloat(tt.flagVal, tt.flagSet, tt.fileVal, tt.def); got != tt.want {
				t.Errorf("ResolveFloat = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestLoad_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(filepath.Join(dir, "does-not-exist.json"))
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
	if cfg.BaseURL != "" || cfg.Model != "" {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	recentN := 7
	conventional := true
	in := FileConfig{
		CompatibleBaseURL: "https://example.com/v1",
		APIKeys:           map[string]string{"compatible": "secret-key"},
		Model:             "gpt-4o",
		Provider:          ProviderCompatible,
		IgnoredFiles:      []string{"*.lock", "dist/"},
		RecentN:           &recentN,
		Conventional:      &conventional,
		ReviewLanguage:    "vi",
	}

	if err := Save(in, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if out.CompatibleBaseURL != in.CompatibleBaseURL || out.APIKeys["compatible"] != "secret-key" || out.Model != in.Model || out.Provider != in.Provider {
		t.Errorf("string fields mismatch: got %+v", out)
	}
	if out.RecentN == nil || *out.RecentN != recentN {
		t.Errorf("RecentN mismatch: got %v", out.RecentN)
	}
	if out.Conventional == nil || *out.Conventional != conventional {
		t.Errorf("Conventional mismatch: got %v", out.Conventional)
	}
	if len(out.IgnoredFiles) != 2 || out.IgnoredFiles[0] != "*.lock" {
		t.Errorf("IgnoredFiles mismatch: got %v", out.IgnoredFiles)
	}
	if out.ReviewLanguage != "vi" {
		t.Errorf("ReviewLanguage mismatch: got %q", out.ReviewLanguage)
	}
}
