package app

import (
	"fmt"
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

func TestTranslateError_KnownSentinels(t *testing.T) {
	tr := i18n.New(i18n.LocaleVI)

	if got := TranslateError(tr, ErrNoStagedChanges); got == "" || got == ErrNoStagedChanges.Error() {
		t.Errorf("expected localized no-staged message, got %q", got)
	}
	if got := TranslateError(tr, &AllFilesIgnoredError{Checked: 3}); got == "" {
		t.Errorf("expected localized ignored message, got %q", got)
	}
	if got := TranslateError(tr, &unknownProviderError{Provider: "weird"}); got == "" {
		t.Error("expected unknown provider message")
	}
	if got := TranslateError(tr, fmt.Errorf("ollama: API error (status 401): Unauthorized")); got == "" || got == "ollama: API error (status 401): Unauthorized" {
		t.Errorf("expected localized ollama unauthorized, got %q", got)
	}
}
