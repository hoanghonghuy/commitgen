package app

import "strings"

// shouldRunSuggestHeadless reports whether suggest must avoid the alt-screen TUI.
// Hook mode always runs headless so git never opens an interactive UI.
func shouldRunSuggestHeadless(cfg Config) bool {
	if cfg.Print || cfg.DryRun {
		return true
	}
	return strings.TrimSpace(cfg.HookFile) != ""
}
