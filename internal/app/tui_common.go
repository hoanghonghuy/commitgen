package app

import (
	"fmt"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/logger"
)

// calcInnerWidth returns usable content width inside the outer border+padding
// (border=2, padding=2 → 4 total), with a minimum of 10.
func calcInnerWidth(width int) int {
	w := width - 4
	if w < 10 {
		w = 10
	}
	return w
}

// calcInnerHeight returns usable content height inside the outer border
// (border top+bottom = 2), with a minimum of 3.
func calcInnerHeight(height int) int {
	h := height - 2
	if h < 3 {
		h = 3
	}
	return h
}

// newSpinnerModel creates a spinner with Dot style and selected styling.
func newSpinnerModel() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleSelected
	return s
}

// newDefaultViewport creates a viewport with the given dimensions ready for use.
func newDefaultViewport(width, height int) viewport.Model {
	return viewport.New(width, height)
}

// clipboardErrMsg is emitted when clipboard write fails.
type clipboardErrMsg struct{ err error }

// clipboardCopyCmd writes content to clipboard and returns a tea.Cmd that
// either signals failure or sets a 1500ms timer before the done message.
func clipboardCopyCmd(content string, onDone tea.Msg) tea.Cmd {
	if err := clipboard.WriteAll(content); err != nil {
		logger.Error("failed to copy to clipboard", "error", err)
		return func() tea.Msg { return clipboardErrMsg{err: err} }
	}
	return tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		return onDone
	})
}

// scrollHintText returns the formatted scroll hint string based on scroll position.
func scrollHintText(tr *i18n.Translator, pct int, atTop, atBottom bool) string {
	if tr == nil {
		tr = i18n.New(i18n.LocaleEN)
	}
	switch {
	case atTop:
		return fmt.Sprintf(tr.T("tui.hint.scroll_top"), pct)
	case atBottom:
		return fmt.Sprintf(tr.T("tui.hint.scroll_bottom"), pct)
	default:
		return fmt.Sprintf(tr.T("tui.hint.scroll_mid"), pct)
	}
}
