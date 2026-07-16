package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/validator"
)

// Sentinel errors allow callers/tests to match on error identity instead of
// fragile string comparisons.
var (
	ErrNoStagedChanges = errors.New("no staged changes")
	ErrNoPRChanges     = errors.New("no commits or file changes since merge-base")
	ErrAllFilesIgnored = errors.New("all staged files were ignored")
	ErrMissingModel    = errors.New("missing model")
	ErrMissingAPIKey   = errors.New("missing api key")
	ErrUnknownProvider = errors.New("unknown provider")
	ErrValidation      = errors.New("commit message validation failed")
)

// AllFilesIgnoredError reports that every staged file was filtered out.
type AllFilesIgnoredError struct {
	Checked int
}

func (e *AllFilesIgnoredError) Error() string {
	return fmt.Sprintf("%s (checked %d files)", ErrAllFilesIgnored.Error(), e.Checked)
}

func (e *AllFilesIgnoredError) Unwrap() error {
	return ErrAllFilesIgnored
}

// ValidationFailedError carries validation issues from the non-interactive path.
type ValidationFailedError struct {
	Issues []validator.Issue
}

func (e *ValidationFailedError) Error() string {
	return ErrValidation.Error()
}

func (e *ValidationFailedError) Unwrap() error {
	return ErrValidation
}

// TranslateError maps sentinel errors to localized user-facing messages.
func TranslateError(tr *i18n.Translator, err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrNoStagedChanges):
		return tr.T("error.no_staged_changes")
	case errors.Is(err, ErrNoPRChanges):
		return tr.T("error.no_pr_changes")
	case errors.Is(err, ErrMissingModel):
		return tr.T("error.missing_model")
	case errors.Is(err, ErrMissingAPIKey):
		return tr.T("error.missing_api_key")
	case errors.Is(err, ErrValidation):
		return tr.T("error.validation_failed")
	}
	var ignored *AllFilesIgnoredError
	if errors.As(err, &ignored) {
		return tr.T("error.all_files_ignored", ignored.Checked)
	}
	var unknown *unknownProviderError
	if errors.As(err, &unknown) {
		return tr.T("error.unknown_provider", unknown.Provider)
	}
	if isOllamaUnauthorized(err) {
		return tr.T("error.ollama_unauthorized")
	}
	return err.Error()
}

func isOllamaUnauthorized(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "ollama") && strings.Contains(msg, "401")
}

type unknownProviderError struct {
	Provider string
}

func (e *unknownProviderError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnknownProvider.Error(), e.Provider)
}

func (e *unknownProviderError) Unwrap() error {
	return ErrUnknownProvider
}

// clampTemperature constrains the LLM temperature to the valid [0, 2] range.
func clampTemperature(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 2 {
		return 2
	}
	return t
}

// clampNonNegative returns 0 for negative values, otherwise n.
func clampNonNegative(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
