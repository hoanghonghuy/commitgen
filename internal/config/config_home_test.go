package config

import (
	"testing"
)

// redirectHome points os.UserHomeDir at a temp dir for the duration of the test.
func redirectHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	// Windows uses USERPROFILE; unix uses HOME.
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
}

func TestLoad_EmptyPathUsesHome(t *testing.T) {
	redirectHome(t)
	// No file exists in the fresh temp home → empty config, no error.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if cfg.Model != "" {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

func TestSaveAndLoad_EmptyPathRoundTrip(t *testing.T) {
	redirectHome(t)

	model := "gpt-4o"
	in := FileConfig{Model: model, Provider: "openai"}
	if err := Save(in, ""); err != nil {
		t.Fatalf("Save(\"\") error: %v", err)
	}

	out, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if out.Model != model || out.Provider != "openai" {
		t.Errorf("round trip mismatch: %+v", out)
	}
}
