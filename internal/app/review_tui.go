package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

var (
	styleReviewTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).MarginLeft(2)
	styleReviewError = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).MarginLeft(2)
)

type reviewState int

const (
	reviewStateAnalyzing reviewState = iota
	reviewStateDone
)

type reviewModel struct {
	state  reviewState
	width  int
	height int

	provider    ai.Provider
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
	switchToSuggest bool // true khi user chọn "Suggest commit message" từ review
}

type reviewResultMsg struct {
	content string
	err     error
}

func newReviewModel(provider ai.Provider, msgs []vscodeprompt.VSCodeMessage, temp float64, timeout time.Duration) reviewModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleSelected

	vp := viewport.New(80, 20)

	return reviewModel{
		state:         reviewStateAnalyzing,
		provider:      provider,
		initialMsgs:   msgs,
		temp:          temp,
		timeout:       timeout,
		spinner:       s,
		viewport:      vp,
		viewportReady: true,
		width:         80,
		height:        24,
	}
}

func (m reviewModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.generateReviewCmd())
}

func (m reviewModel) generateReviewCmd() tea.Cmd {
	return func() tea.Msg {
		currentMsgs := make([]vscodeprompt.VSCodeMessage, len(m.initialMsgs))
		copy(currentMsgs, m.initialMsgs)

		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()

		raw, err := m.provider.Generate(ctx, currentMsgs, m.temp)
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
	w := m.width - 4
	if w < 10 {
		w = 10
	}
	return w
}

func (m reviewModel) innerHeight() int {
	h := m.height - 2
	if h < 3 {
		h = 3
	}
	return h
}

func (m reviewModel) buildDoneContent() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(styleReviewTitle.Render("Review Report"))
	b.WriteString("\n")
	b.WriteString(msgContentStyle(m.innerWidth() - 6).Render(m.report))
	b.WriteString("\n\n")

	b.WriteString(styleActionTitle.Render("Action"))
	b.WriteString("\n")

	options := []string{"Suggest commit message", "Regenerate", "Exit"}
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
	if m.state != reviewStateDone || m.report == "" {
		return m
	}

	content := m.buildDoneContent()
	m.cachedContent = content
	totalLines := countLines(content)
	m.needsScroll = totalLines > m.innerHeight()

	if m.needsScroll && m.viewportReady {
		m.viewport.SetContent(content)

		// Auto-scroll to keep cursor action item in view.
		// Action lines are at the end of content (3 items: Suggest, Regenerate, Exit):
		//   cursor=0 → 3rd from end, cursor=1 → 2nd, cursor=2 → last
		lineFromEnd := 3 - m.cursor
		cursorLine := totalLines - 1 - lineFromEnd

		viewTop := m.viewport.YOffset
		viewBottom := m.viewport.YOffset + m.viewport.Height - 1
		if cursorLine < viewTop {
			m.viewport.SetYOffset(cursorLine)
		} else if cursorLine > viewBottom {
			m.viewport.SetYOffset(cursorLine - m.viewport.Height + 1)
		}
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
		case reviewStateDone:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					m = m.refreshViewport()
				}
			case "down", "j":
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
		if m.state == reviewStateDone && m.needsScroll && m.viewportReady {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		vpHeight := m.innerHeight()
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
		m.state = reviewStateDone
		m.cursor = 0
		m = m.refreshViewport()
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
		inner = fmt.Sprintf("\n %s Analyzing staged changes...\n", m.spinner.View())

	case reviewStateDone:
		if m.err != nil {
			inner = fmt.Sprintf("\n %s\n", styleReviewError.Render("Error: "+m.err.Error()))
		} else if m.needsScroll && m.viewportReady {
			pct := int(m.viewport.ScrollPercent() * 100)
			var hint string
			switch {
			case m.viewport.AtTop():
				hint = fmt.Sprintf(" ↓ PgDn/Scroll  %d%% ", pct)
			case m.viewport.AtBottom():
				hint = fmt.Sprintf(" ↑ PgUp/Scroll  %d%% ", pct)
			default:
				hint = fmt.Sprintf(" ↑↓ PgUp/PgDn  %d%% ", pct)
			}
			inner = m.viewport.View() + "\n" + styleHint.Render(hint)
		} else if m.cachedContent != "" {
			inner = m.cachedContent
		} else {
			inner = m.buildDoneContent()
		}
	}

	if inner == "" {
		return ""
	}

	ws := styleWindow.Width(m.width - 2)
	if m.needsScroll && m.height > 0 {
		ws = ws.Height(m.height - 2)
	}

	return ws.Render(inner)
}
