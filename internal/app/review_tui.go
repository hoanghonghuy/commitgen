package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

var (
	styleReviewTitle  = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).MarginLeft(2)
	styleReviewError  = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).MarginLeft(2)
	styleReviewH2     = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).MarginLeft(2) // yellow bold for ## headings
	styleReviewH3     = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true).MarginLeft(4) // cyan for ### headings
	styleReviewCode   = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Padding(0, 1)            // bright cyan for inline code
	styleReviewQuote  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).MarginLeft(2)           // gray for > blockquote
	styleReviewRule   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))                         // dim for ---
	styleReviewBold   = lipgloss.NewStyle().Bold(true)
	styleReviewBorder = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("237")).
				PaddingLeft(1)
	reMDH2           = regexp.MustCompile(`^##\s+(.+)$`)
	reMDH3           = regexp.MustCompile(`^###\s+(.+)$`)
	reMDBold         = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMDInlineCode   = regexp.MustCompile("`([^`]+?)`")
	reMDBullet       = regexp.MustCompile(`^[-*]\s+`)
	reMDNumberedList = regexp.MustCompile(`^\d+\.\s+`)
	reMDQuote        = regexp.MustCompile(`^>\s*`)
	reMDRule         = regexp.MustCompile(`^(-{3,}|\*{3,})$`)
)

// formatReviewText converts simple markdown to terminal-friendly formatted text.
func formatReviewText(s string) string {
	lines := strings.Split(s, "\n")
	var b strings.Builder
	prevBlank := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// strip code block markers
		if strings.HasPrefix(trimmed, "```") {
			continue
		}
		if trimmed == "" {
			if !prevBlank {
				b.WriteString("\n")
			}
			prevBlank = true
			continue
		}
		prevBlank = false

		// --- or *** → horizontal rule
		if reMDRule.MatchString(trimmed) {
			b.WriteString(styleReviewRule.Render(strings.Repeat("─", 50)))
			b.WriteString("\n")
			continue
		}

		// ## Heading → styled heading
		if m := reMDH2.FindStringSubmatch(trimmed); len(m) == 2 {
			b.WriteString(styleReviewH2.Render(m[1]))
			b.WriteString("\n")
			continue
		}

		// ### Heading → styled heading (level 3)
		if m := reMDH3.FindStringSubmatch(trimmed); len(m) == 2 {
			b.WriteString(styleReviewH3.Render(m[1]))
			b.WriteString("\n")
			continue
		}

		// > blockquote
		if reMDQuote.MatchString(trimmed) {
			text := reMDQuote.ReplaceAllString(trimmed, "")
			text = applyInlineStyles(text)
			b.WriteString(styleReviewQuote.Render("│ " + text))
			b.WriteString("\n")
			continue
		}

		// - item → bullet
		if reMDBullet.MatchString(trimmed) {
			text := reMDBullet.ReplaceAllString(trimmed, "• ")
			text = applyInlineStyles(text)
			b.WriteString(text)
			b.WriteString("\n")
			continue
		}

		// 1. numbered list
		if reMDNumberedList.MatchString(trimmed) {
			text := applyInlineStyles(trimmed)
			b.WriteString(text)
			b.WriteString("\n")
			continue
		}

		// plain line with inline styles
		b.WriteString(applyInlineStyles(trimmed))
		b.WriteString("\n")
	}
	return b.String()
}

// applyInlineStyles handles inline code and bold in a single line.
// NOTE: We intentionally skip italic (*text* / _text_) because:
//   - It conflicts with **bold** regex in terminals
//   - Italic is hard to read in most terminal fonts
//   - AI review output rarely needs italic emphasis
func applyInlineStyles(s string) string {
	// 1. inline code `text` — handle first since it's most specific
	s = reMDInlineCode.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[1 : len(match)-1]
		return styleReviewCode.Render(inner)
	})
	// 2. bold **text**
	s = reMDBold.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-2]
		return styleReviewBold.Render(inner)
	})
	return s
}

type reviewState int

const (
	reviewStateAnalyzing reviewState = iota
	reviewStateQuickDone
	reviewStateDone
	reviewStateCopied
)

const (
	reviewActionCount = 4 // max number of options in review action menu (quick done has 4)
)

type reviewModel struct {
	state  reviewState
	width  int
	height int

	provider    ai.Provider
	runCtx      context.Context
	initialMsgs []vscodeprompt.VSCodeMessage
	temp        float64
	timeout     time.Duration

	spinner       spinner.Model
	viewport      viewport.Model
	viewportReady bool
	needsScroll   bool

	report          string
	cachedContent   string
	cursor          int
	err             error
	quitting        bool
	switchToSuggest bool // true when user selects "Suggest commit message" from review
	isQuickMode     bool // true when current report is from quick review

	// fullReviewSystem is the system message used when switching from a quick
	// scan to the full review ("View Details"). It is pre-built so a custom
	// prompt template is honored; when nil the built-in default is used.
	fullReviewSystem *vscodeprompt.VSCodeMessage

	tr *i18n.Translator
}

type reviewResultMsg struct {
	content string
	err     error
}

// reviewCopyDoneMsg is sent after clipboard copy feedback expires.
type reviewCopyDoneMsg struct{}

func newReviewModel(ctx context.Context, provider ai.Provider, msgs []vscodeprompt.VSCodeMessage, temp float64, timeout time.Duration, quickMode bool, tr *i18n.Translator) reviewModel {
	if ctx == nil {
		ctx = context.Background()
	}
	s := newSpinnerModel()

	vp := newDefaultViewport(80, 20)

	return reviewModel{
		state:         reviewStateAnalyzing,
		runCtx:        ctx,
		provider:      provider,
		initialMsgs:   msgs,
		temp:          temp,
		timeout:       timeout,
		spinner:       s,
		viewport:      vp,
		viewportReady: true,
		width:         80,
		height:        24,
		isQuickMode:   quickMode,
		tr:            tr,
	}
}

func (m reviewModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.generateReviewCmd())
}

func (m reviewModel) generateReviewCmd() tea.Cmd {
	return func() tea.Msg {
		currentMsgs := make([]vscodeprompt.VSCodeMessage, len(m.initialMsgs))
		copy(currentMsgs, m.initialMsgs)

		ctx, cancel := context.WithTimeout(m.runCtx, m.timeout)
		defer cancel()

		raw, err := m.provider.Generate(ctx, currentMsgs, clampTemperature(m.temp))
		if err != nil {
			logger.Error("failed to generate review", "error", err)
			return reviewResultMsg{err: err}
		}

		report, ok := vscodeprompt.ExtractOneTextCodeBlock(raw)
		if !ok {
			report = raw
		}
		return reviewResultMsg{content: report}
	}
}

func (m reviewModel) innerWidth() int {
	return calcInnerWidth(m.width)
}

func (m reviewModel) innerHeight() int {
	return calcInnerHeight(m.height)
}

func (m reviewModel) buildDoneContent() string {
	var b strings.Builder

	b.WriteString("\n")
	if m.isQuickMode {
		b.WriteString(styleReviewTitle.Render(m.tr.T("tui.review.quick_scan")))
	} else {
		b.WriteString(styleReviewTitle.Render(m.tr.T("tui.review.report")))
	}
	b.WriteString("\n")

	// Wrap and border the review report.
	// We use ansi.Wrap (ANSI-aware) before applying border,
	// so long lines are wrapped correctly without breaking styles.
	w := m.innerWidth() - 4 // account for border + padding
	wrapped := ansi.Wrap(formatReviewText(m.report), w, " ")
	b.WriteString(styleReviewBorder.Render(wrapped))
	b.WriteString("\n\n")

	b.WriteString(styleActionTitle.Render(m.tr.T("tui.title.action")))
	b.WriteString("\n")

	var options []string
	if m.isQuickMode {
		options = []string{m.tr.T("tui.action.view_details"), m.tr.T("tui.action.suggest_commit"), m.tr.T("tui.action.regenerate"), m.tr.T("tui.action.exit")}
	} else {
		options = []string{m.tr.T("tui.action.suggest_commit"), m.tr.T("tui.action.regenerate"), m.tr.T("tui.action.exit")}
	}
	barStr := styleBar.Render("┃")
	for i, opt := range options {
		if m.cursor == i {
			b.WriteString(fmt.Sprintf("%s > %s\n", barStr, styleSelected.Render(opt)))
		} else {
			b.WriteString(fmt.Sprintf("%s   %s\n", barStr, opt))
		}
	}

	return b.String()
}

func (m reviewModel) refreshViewport() reviewModel {
	if (m.state != reviewStateDone && m.state != reviewStateQuickDone) || m.report == "" {
		return m
	}

	content := m.buildDoneContent()
	m.cachedContent = content
	m.needsScroll = true // always use viewport for review report (handles word-wrap)

	if m.viewportReady {
		m.viewport.GotoTop() // ensure we start at the top
		m.viewport.SetContent(content)
	}
	return m
}

func (m reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

		switch m.state {
		case reviewStateQuickDone:
			switch msg.String() {
			case "y", "Y":
				if m.report != "" {
					// Copy plain report text to clipboard
					if cmd := clipboardCopyCmd(m.report, reviewCopyDoneMsg{}); cmd != nil {
						m.state = reviewStateCopied
						return m, cmd
					}
				}
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					m = m.refreshViewport()
				}
			case "down", "j":
				// Quick done has 4 options
				if m.cursor < 3 {
					m.cursor++
					m = m.refreshViewport()
				}
			case "pgup":
				if m.needsScroll {
					m.viewport.HalfViewUp()
				}
			case "pgdown":
				if m.needsScroll {
					m.viewport.HalfViewDown()
				}
			case "enter":
				switch m.cursor {
				case 0: // View Details → full review
					m.isQuickMode = false
					m.state = reviewStateAnalyzing
					m.report = ""
					m.cachedContent = ""
					// Rebuild messages: keep user message (contains diff data), replace
					// system with the full-review prompt. Prefer the pre-built system
					// message (which honors a custom prompt template) and fall back to
					// the built-in default when none was provided.
					fullSystem := vscodeprompt.VSCodeMessage{
						Role: vscodeprompt.RoleSystem,
						Content: []vscodeprompt.VSCodeContentPart{
							{Type: 1, Text: vscodeprompt.DefaultFullReviewPromptTemplate()},
						},
					}
					if m.fullReviewSystem != nil {
						fullSystem = *m.fullReviewSystem
					}
					if len(m.initialMsgs) >= 2 {
						m.initialMsgs[0] = fullSystem
					}
					return m, tea.Batch(m.spinner.Tick, m.generateReviewCmd())
				case 1: // Suggest commit message
					m.switchToSuggest = true
					m.quitting = true
					return m, tea.Quit
				case 2: // Regenerate (quick review again)
					m.state = reviewStateAnalyzing
					m.report = ""
					m.cachedContent = ""
					m.isQuickMode = true
					return m, tea.Batch(m.spinner.Tick, m.generateReviewCmd())
				case 3: // Exit
					m.quitting = true
					return m, tea.Quit
				}
			}
		case reviewStateDone:
			switch msg.String() {
			case "y", "Y":
				if m.report != "" {
					// Copy plain report text to clipboard
					if cmd := clipboardCopyCmd(m.report, reviewCopyDoneMsg{}); cmd != nil {
						m.state = reviewStateCopied
						return m, cmd
					}
				}
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					m = m.refreshViewport()
				}
			case "down", "j":
				// Full done has 3 options
				if m.cursor < 2 {
					m.cursor++
					m = m.refreshViewport()
				}
			case "pgup":
				if m.needsScroll {
					m.viewport.HalfViewUp()
				}
			case "pgdown":
				if m.needsScroll {
					m.viewport.HalfViewDown()
				}
			case "enter":
				switch m.cursor {
				case 0: // Suggest commit message
					m.switchToSuggest = true
					m.quitting = true
					return m, tea.Quit
				case 1: // Regenerate
					m.state = reviewStateAnalyzing
					m.report = ""
					m.cachedContent = ""
					return m, tea.Batch(m.spinner.Tick, m.generateReviewCmd())
				case 2: // Exit
					m.quitting = true
					return m, tea.Quit
				}
			}
		}

	case tea.MouseMsg:
		if (m.state == reviewStateDone || m.state == reviewStateQuickDone) && m.needsScroll && m.viewportReady {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Viewport height must account for the border (2 lines), padding (2 lines),
		// and the scroll hint line (1 line) that follows viewport.View().
		// Total overhead = 5 lines. innerHeight() already subtracts border (2),
		// so we need to subtract 3 more (padding 2 + hint 1).
		vpHeight := m.innerHeight() - 3
		if vpHeight < 3 {
			vpHeight = 3
		}
		if m.viewportReady {
			m.viewport.Width = m.innerWidth()
			m.viewport.Height = vpHeight
		} else {
			m.viewport = viewport.New(m.innerWidth(), vpHeight)
			m.viewportReady = true
		}
		m = m.refreshViewport()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case reviewResultMsg:
		if msg.err != nil {
			logger.Error("review generation failed", "error", msg.err)
			m.err = msg.err
			m.state = reviewStateDone
			return m, tea.Quit
		}
		m.report = msg.content
		if m.isQuickMode {
			m.state = reviewStateQuickDone
		} else {
			m.state = reviewStateDone
		}
		m.cursor = 0
		m.viewport.GotoTop() // reset scroll to top for new content
		m = m.refreshViewport()
		return m, nil

	case reviewCopyDoneMsg:
		m.state = reviewStateDone
		return m, nil
	}

	return m, nil
}

func (m reviewModel) View() string {
	if m.quitting {
		return ""
	}

	var inner string

	switch m.state {
	case reviewStateAnalyzing:
		inner = fmt.Sprintf("\n %s %s\n", m.spinner.View(), m.tr.T("tui.hint.analyzing"))

	case reviewStateDone:
		if m.err != nil {
			inner = fmt.Sprintf("\n %s\n", styleReviewError.Render(m.tr.T("tui.state.error", m.err)))
		} else if m.viewportReady {
			pct := int(m.viewport.ScrollPercent() * 100)
			hint := scrollHintText(pct, m.viewport.AtTop(), m.viewport.AtBottom())
			inner = m.viewport.View() + "\n" + styleHint.Render(hint)
		} else if m.cachedContent != "" {
			inner = m.cachedContent
		} else {
			inner = m.buildDoneContent()
		}

	case reviewStateQuickDone:
		if m.err != nil {
			inner = fmt.Sprintf("\n %s\n", styleReviewError.Render(m.tr.T("tui.state.error", m.err)))
		} else if m.viewportReady {
			pct := int(m.viewport.ScrollPercent() * 100)
			hint := scrollHintText(pct, m.viewport.AtTop(), m.viewport.AtBottom())
			inner = m.viewport.View() + "\n" + styleHint.Render(hint)
		} else if m.cachedContent != "" {
			inner = m.cachedContent
		} else {
			inner = m.buildDoneContent()
		}

	case reviewStateCopied:
		inner = fmt.Sprintf("\n  %s\n", m.tr.T("tui.state.copied"))
	}

	if inner == "" {
		return ""
	}

	ws := styleWindow.Width(m.width - 2)

	return ws.Render(inner)
}
