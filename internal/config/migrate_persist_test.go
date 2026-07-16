package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EagerRewriteLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")
	raw := `{"provider":"openai","base_url":"https://openrouter.ai/api/v1","api_key":"or-key","model":"x"}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != ProviderOpenRouter {
		t.Fatalf("Provider=%q", cfg.Provider)
	}
	if cfg.APIKeys["openrouter"] != "or-key" {
		t.Fatalf("api_keys=%v", cfg.APIKeys)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk FileConfig
	if err := json.Unmarshal(b, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.Provider != ProviderOpenRouter {
		t.Errorf("disk provider=%q", disk.Provider)
	}
	if disk.APIKey != "" || disk.BaseURL != "" {
		t.Errorf("disk still has legacy fields: api_key=%q base_url=%q", disk.APIKey, disk.BaseURL)
	}
	if disk.APIKeys["openrouter"] != "or-key" {
		t.Errorf("disk api_keys=%v", disk.APIKeys)
	}
}

func TestLoad_AlreadyMigratedNotRewritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")
	in := FileConfig{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o",
		APIKeys:  map[string]string{"openai": "k"},
	}
	if err := Save(in, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("file content changed unexpectedly\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		// Content equal is enough; some FS may touch mtime — only fail if content differs.
	}
}

func TestMerge_UnionsAPIKeys(t *testing.T) {
	base := FileConfig{
		Provider: ProviderOpenAI,
		APIKeys:  map[string]string{"openai": "oa", "openrouter": "or"},
	}
	override := FileConfig{
		Provider: ProviderGemini,
		APIKeys:  map[string]string{"gemini": "gem", "openrouter": "or2"},
	}
	out := Merge(base, override)
	if out.APIKeys["openai"] != "oa" {
		t.Errorf("openai key dropped: %v", out.APIKeys)
	}
	if out.APIKeys["openrouter"] != "or2" {
		t.Errorf("openrouter override failed: %v", out.APIKeys)
	}
	if out.APIKeys["gemini"] != "gem" {
		t.Errorf("gemini missing: %v", out.APIKeys)
	}
	if out.Provider != ProviderGemini {
		t.Errorf("Provider=%q", out.Provider)
	}
}
