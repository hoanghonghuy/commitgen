package app

import (
	"fmt"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

// clipboardCopyCmd writes content to clipboard and returns a tea.Cmd that
// sets a 1500ms timer before signaling the done message.
func clipboardCopyCmd(content string, onDone tea.Msg) tea.Cmd {
	if err := clipboard.WriteAll(content); err != nil {
		logger.Error("failed to copy to clipboard", "error", err)
		return nil
	}
	return tea.Tick(1500*time.Millisecond, func(_ time.Time) tea.Msg {
		return onDone
	})
}

// scrollHintText returns the formatted scroll hint string based on scroll position.
func scrollHintText(pct int, atTop, atBottom bool) string {
	switch {
	case atTop:
		return fmt.Sprintf(" ↓ PgDn/Scroll  %d%%  |  y Copy ", pct)
	case atBottom:
		return fmt.Sprintf(" ↑ PgUp/Scroll  %d%%  |  y Copy ", pct)
	default:
		return fmt.Sprintf(" ↑↓ PgUp/PgDn  %d%%  |  y Copy ", pct)
	}
}
