package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMerge_OverrideWins(t *testing.T) {
	n5, n9 := 5, 9
	tru := true
	base := FileConfig{
		CompatibleBaseURL: "https://base",
		Model:             "base-model",
		Provider:          "openai",
		RecentN:           &n5,
		Conventional:      &tru,
		IgnoredFiles:      []string{"a"},
	}
	override := FileConfig{
		Model:        "override-model",
		RecentN:      &n9,
		IgnoredFiles: []string{"b", "c"},
	}
	out := Merge(base, override)

	if out.Model != "override-model" {
		t.Errorf("Model = %q; want override", out.Model)
	}
	if out.CompatibleBaseURL != "https://base" {
		t.Errorf("CompatibleBaseURL should stay base, got %q", out.CompatibleBaseURL)
	}
	if out.RecentN == nil || *out.RecentN != 9 {
		t.Errorf("RecentN = %v; want 9", out.RecentN)
	}
	if out.Conventional == nil || !*out.Conventional {
		t.Error("Conventional should be preserved from base")
	}
	if len(out.IgnoredFiles) != 2 {
		t.Errorf("IgnoredFiles should be overridden, got %v", out.IgnoredFiles)
	}
}

func TestMerge_EmptyOverrideKeepsBase(t *testing.T) {
	base := FileConfig{CompatibleBaseURL: "https://base", Model: "m", Provider: "openai", ReviewLanguage: "vi"}
	out := Merge(base, FileConfig{})
	if out.CompatibleBaseURL != "https://base" || out.Model != "m" || out.ReviewLanguage != "vi" {
		t.Errorf("empty override should keep base, got %+v", out)
	}
}

func TestLoadResolved_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "explicit.json")
	if err := Save(FileConfig{Model: "explicit-model"}, p); err != nil {
		t.Fatal(err)
	}
	out, err := LoadResolved(p)
	if err != nil {
		t.Fatalf("LoadResolved error: %v", err)
	}
	if out.Model != "explicit-model" {
		t.Errorf("Model = %q", out.Model)
	}
}

func TestLoadResolved_RepoLocalOverlaysGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	// Global config in home.
	if err := Save(FileConfig{Model: "global-model", Provider: "openai", CompatibleBaseURL: "https://global"}, ""); err != nil {
		t.Fatal(err)
	}

	// Repo-local config in a working directory.
	repo := t.TempDir()
	local := filepath.Join(repo, ".commitgen.json")
	if err := os.WriteFile(local, []byte(`{"model":"local-model"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	out, err := LoadResolved("")
	if err != nil {
		t.Fatalf("LoadResolved error: %v", err)
	}
	if out.Model != "local-model" {
		t.Errorf("repo-local should override Model, got %q", out.Model)
	}
	if out.CompatibleBaseURL != "https://global" {
		t.Errorf("global CompatibleBaseURL should remain, got %q", out.CompatibleBaseURL)
	}
	if got, ok := RepoLocalConfigPath(); !ok || got != local {
		t.Errorf("RepoLocalConfigPath=(%q,%v); want (%q,true)", got, ok, local)
	}
}

func TestLoadResolved_NoRepoLocalUsesGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if err := Save(FileConfig{Model: "global-only"}, ""); err != nil {
		t.Fatal(err)
	}
	// cwd is the home dir itself: the only .commitgen.json found is the global
	// one (excluded), and the search is bounded at home.
	t.Chdir(home)

	out, err := LoadResolved("")
	if err != nil {
		t.Fatalf("LoadResolved error: %v", err)
	}
	if out.Model != "global-only" {
		t.Errorf("Model = %q; want global-only", out.Model)
	}
	if got, ok := RepoLocalConfigPath(); ok || got != "" {
		t.Errorf("RepoLocalConfigPath=(%q,%v); want empty,false", got, ok)
	}
}
