package app

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// outcomeHoldDuration is how long success/error screens stay visible before Quit.
// Tests may set this to 0 for an immediate outcomeHoldDoneMsg.
var outcomeHoldDuration = 1500 * time.Millisecond

// outcomeHoldDoneMsg fires after the success/error hold completes.
type outcomeHoldDoneMsg struct{}

func outcomeHoldCmd() tea.Cmd {
	d := outcomeHoldDuration
	if d <= 0 {
		return func() tea.Msg { return outcomeHoldDoneMsg{} }
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return outcomeHoldDoneMsg{} })
}

// printDurableOutcome writes a post-alt-screen outcome line (design fallback for REQ-TUI-03).
// Call only when the TUI reached a terminal done/error outcome (not plain cancel/quit).
func printDurableOutcome(w io.Writer, tr *i18n.Translator, err error) {
	if w == nil {
		w = os.Stderr
	}
	if tr == nil {
		tr = i18n.New(i18n.LocaleEN)
	}
	if err != nil {
		fmt.Fprintln(w, tr.T("tui.state.error", err))
		return
	}
	fmt.Fprintln(w, tr.T("tui.state.success"))
}

// actionFooter returns the standard nav footer for menu states.
func actionFooter(tr *i18n.Translator) string {
	if tr == nil {
		tr = i18n.New(i18n.LocaleEN)
	}
	return tr.T("tui.hint.action_footer")
}

// statusLine renders a short status banner (errors / no-ops).
func statusLine(tr *i18n.Translator, msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if tr == nil {
		return " ! " + msg + "\n"
	}
	return styleHint.Render(" ! "+msg) + "\n"
}

// candidatePreview returns up to maxLines of a candidate for the choose UI.
func candidatePreview(msg string, maxLines int) string {
	if maxLines < 1 {
		maxLines = 1
	}
	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	out := strings.Join(lines[:maxLines], "\n")
	return out + "\n…"
}

// candidatesProgress formats generating progress for --count > 1.
func candidatesProgress(tr *i18n.Translator, current, total int) string {
	if tr == nil {
		tr = i18n.New(i18n.LocaleEN)
	}
	return tr.T("tui.hint.candidates_progress", current, total)
}

// shouldUseAccessibleForm enables huh accessible mode for screen readers / dumb terminals.
func shouldUseAccessibleForm(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	if strings.TrimSpace(getenv("ACCESSIBLE")) != "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(getenv("TERM")), "dumb")
}

// appendGuidanceMessage adds optional regenerate guidance as a user message.
func appendGuidanceMessage(msgs []vscodeprompt.VSCodeMessage, tr *i18n.Translator, hint string) []vscodeprompt.VSCodeMessage {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return msgs
	}
	if tr == nil {
		tr = i18n.New(i18n.LocaleEN)
	}
	prefix := tr.T("tui.regen.guidance_prefix")
	out := make([]vscodeprompt.VSCodeMessage, len(msgs), len(msgs)+1)
	copy(out, msgs)
	return append(out, vscodeprompt.VSCodeMessage{
		Role:    vscodeprompt.RoleUser,
		Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: prefix + hint}},
	})
}

// formatGenProgress is used while multi-candidate generation is in flight.
func formatGenProgress(tr *i18n.Translator, spinnerView string, current, total int) string {
	if tr == nil {
		tr = i18n.New(i18n.LocaleEN)
	}
	if total <= 1 {
		return fmt.Sprintf("\n %s %s\n", spinnerView, tr.T("tui.hint.generating"))
	}
	return fmt.Sprintf("\n %s %s\n", spinnerView, candidatesProgress(tr, current, total))
}

// applyFormAccessibility enables huh accessible mode when env says so.
func applyFormAccessibility(form *huh.Form, getenv func(string) string) *huh.Form {
	if form == nil {
		return nil
	}
	return form.WithAccessible(shouldUseAccessibleForm(getenv))
}

// validateIntField maps strconv failures to a localized error.
func validateIntField(tr *i18n.Translator, s string) error {
	if _, err := strconv.Atoi(strings.TrimSpace(s)); err != nil {
		if tr == nil {
			tr = i18n.New(i18n.LocaleEN)
		}
		return fmt.Errorf("%s", tr.T("config.field.integer.error"))
	}
	return nil
}
