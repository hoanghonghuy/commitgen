package i18n

import (
	"testing"
)

func TestNewTranslator(t *testing.T) {
	tr := New(LocaleEN)
	if tr == nil {
		t.Fatal("New() returned nil")
	}
	if tr.locale != LocaleEN {
		t.Errorf("expected locale %q, got %q", LocaleEN, tr.locale)
	}
}

func TestTranslator_T_FallbackToEnglish(t *testing.T) {
	tr := New(LocaleVI)
	// "tui.title.action" exists in en.json but may not exist in vi.json
	// The translator should fall back to English.
	got := tr.T("tui.title.action")
	if got == "" || got == "tui.title.action" {
		t.Errorf("T() should fall back to English, got %q", got)
	}
}

func TestTranslator_T_ReturnsKeyWhenMissing(t *testing.T) {
	tr := New(LocaleEN)
	got := tr.T("nonexistent.key.xyz")
	if got != "nonexistent.key.xyz" {
		t.Errorf("expected key itself when missing, got %q", got)
	}
}

func TestTranslator_T_WithArgs(t *testing.T) {
	tr := New(LocaleEN)
	// "error.all_files_ignored" expects %d
	got := tr.T("error.all_files_ignored", 5)
	if got == "error.all_files_ignored" {
		t.Errorf("T() with args should format the message, got key back")
	}
}

func TestTranslator_T_EnglishDirect(t *testing.T) {
	tr := New(LocaleEN)
	got := tr.T("tui.title.generated_message")
	if got != "Generated Commit Message" {
		t.Errorf("expected 'Generated Commit Message', got %q", got)
	}
}

func TestTranslator_T_Vietnamese(t *testing.T) {
	tr := New(LocaleVI)
	got := tr.T("tui.title.generated_message")
	if got != "Commit Message Được Tạo" {
		t.Errorf("expected Vietnamese translation, got %q", got)
	}
}

func TestTranslator_Locale(t *testing.T) {
	tr := New(LocaleJA)
	if tr.Locale() != LocaleJA {
		t.Errorf("expected Locale() to return %q, got %q", LocaleJA, tr.Locale())
	}
}

func TestAllLocalesHaveSameKeys(t *testing.T) {
	// Ensure all locale files have the same set of keys as English.
	enKeys := keysForLocale(LocaleEN)
	if len(enKeys) == 0 {
		t.Fatal("English messages are empty — test cannot proceed")
	}

	for _, loc := range []Locale{LocaleVI, LocaleJA, LocaleZH} {
		locKeys := keysForLocale(loc)
		for k := range enKeys {
			if _, ok := locKeys[k]; !ok {
				t.Errorf("locale %q is missing key %q", loc, k)
			}
		}
	}
}
