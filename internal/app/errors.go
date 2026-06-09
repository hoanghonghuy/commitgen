package app

import "errors"

// Sentinel errors allow callers/tests to match on error identity instead of
// fragile string comparisons.
var (
	ErrNoStagedChanges = errors.New("no staged changes")
	ErrAllFilesIgnored = errors.New("all staged files were ignored")
	ErrMissingModel    = errors.New("missing model")
	ErrMissingAPIKey   = errors.New("missing api key")
	ErrUnknownProvider = errors.New("unknown provider")
)

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
