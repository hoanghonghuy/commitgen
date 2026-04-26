package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// Pre-computed styles — allocated once at startup, not on every frame.
var (
	styleMsgTitle    = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).MarginLeft(2)
	styleActionTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).MarginLeft(2)
	styleBar         = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	styleSelected    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	styleHint        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleEditTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true).MarginLeft(2)
	styleWindow      = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("245")).
				Padding(0, 1)
)

// msgContentStyle is width-dependent so it's a helper, not a global var.
func msgContentStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("237")).
		PaddingLeft(1).
		Width(width)
}

type tuiState int

const (
	stateGenerating  tuiState = iota // AI đang tạo commit message
	stateCommitting                  // Đang thực hiện git commit
	stateConfirm
	stateEditing
	stateDone
)

type tuiModel struct {
	state  tuiState
	width  int
	height int

	// Dependencies
	provider     ai.Provider
	initialMsgs  []vscodeprompt.VSCodeMessage
	temp         float64
	timeout      time.Duration
	conventional bool
	hookFile     string
	repoRoot     string

	// Components
	spinner       spinner.Model
	textarea      textarea.Model
	viewport      viewport.Model
	viewportReady bool
	needsScroll   bool // true khi content vượt quá inner height

	// Data
	commitMsg      string
	cachedContent  string // built once in Update, read in View — avoids per-frame rebuild
	cursor         int
	err            error
	quitting       bool
}

type commitResultMsg struct {
	content string
	err     error
}

type commitDoneMsg struct {
	err error
}

func newTuiModel(repoRoot string, provider ai.Provider, msgs []vscodeprompt.VSCodeMessage, temp float64, timeout time.Duration, conventional bool, hookFile string) tuiModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleSelected // reuse pre-computed style

	ta := textarea.New()
	ta.Placeholder = "Enter commit message..."
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(5)

	return tuiModel{
		state:        stateGenerating,
		provider:     provider,
		initialMsgs:  msgs,
		temp:         temp,
		timeout:      timeout,
		conventional: conventional,
		hookFile:     hookFile,
		repoRoot:     repoRoot,
		spinner:      s,
		textarea:     ta,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.generateCommitCmd())
}

func (m tuiModel) generateCommitCmd() tea.Cmd {
	return func() tea.Msg {
		logger.Debug("generating commit message", "conventional", m.conventional)
		currentMsgs := make([]vscodeprompt.VSCodeMessage, len(m.initialMsgs))
		copy(currentMsgs, m.initialMsgs)

		if m.conventional {
			reminderMsg := vscodeprompt.VSCodeMessage{
				Role: vscodeprompt.RoleUser,
				Content: []vscodeprompt.VSCodeContentPart{
					{Type: 1, Text: "CRITICAL INSTRUCTION: You must strictly follow the Conventional Commits specification (e.g. 'feat: add spinner', 'fix: resolve bug').\nDo not just describe the change; prefix it with the type."},
				},
			}
			currentMsgs = append(currentMsgs, reminderMsg)
		}

		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()

		raw, err := m.provider.GenerateCommitMessage(ctx, currentMsgs, m.temp)
		if err != nil {
			logger.Error("failed to generate commit message", "error", err)
			return commitResultMsg{err: err}
		}

		logger.Debug("commit message generated", "length", len(raw))
		msg, ok := vscodeprompt.ExtractOneTextCodeBlock(raw)
		if !ok {
			logger.Warn("no code block found in response, using raw output")
			msg = raw
		}
		return commitResultMsg{content: msg}
	}
}

func (m tuiModel) commitCmd() tea.Cmd {
	return func() tea.Msg {
		logger.Info("committing changes", "hook_mode", m.hookFile != "")
		if m.hookFile != "" {
			err := os.WriteFile(m.hookFile, []byte(m.commitMsg), 0644)
			if err != nil {
				logger.Error("failed to write hook file", "error", err, "path", m.hookFile)
			}
			return commitDoneMsg{err: err}
		}
		err := gitx.Commit(context.Background(), m.repoRoot, m.commitMsg)
		if err != nil {
			logger.Error("git commit failed", "error", err)
		} else {
			logger.Info("commit successful")
		}
		return commitDoneMsg{err: err}
	}
}

// innerWidth returns usable width inside the outer border+padding (border=2, padding=2 → 4 total).
func (m tuiModel) innerWidth() int {
	w := m.width - 4
	if w < 10 {
		w = 10
	}
	return w
}

// innerHeight returns usable height inside the outer border (border top+bottom = 2).
func (m tuiModel) innerHeight() int {
	h := m.height - 2
	if h < 3 {
		h = 3
	}
	return h
}

// countLines counts the number of terminal lines in s.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// buildConfirmContent builds the full string for stateConfirm.
// Uses pre-computed package-level styles where possible.
// Called from Update() only — result is cached in m.cachedContent.
func (m tuiModel) buildConfirmContent() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(styleMsgTitle.Render("Generated Commit Message"))
	b.WriteString("\n")
	b.WriteString(msgContentStyle(m.innerWidth() - 6).Render(m.commitMsg))
	b.WriteString("\n\n") // blank line before Action section

	b.WriteString(styleActionTitle.Render("Action"))
	b.WriteString("\n")

	options := []string{"Commit (Apply)", "Regenerate", "Edit", "Cancel"}
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

// refreshViewport rebuilds confirm content, caches it, updates viewport + needsScroll,
// and auto-scrolls to keep the current action cursor visible.
// Must be called from Update() only (modifies model state).
func (m tuiModel) refreshViewport() tuiModel {
	if !m.viewportReady || m.state != stateConfirm || m.commitMsg == "" {
		return m
	}
	content := m.buildConfirmContent()
	m.cachedContent = content
	totalLines := countLines(content)
	m.needsScroll = totalLines > m.innerHeight()

	if m.needsScroll {
		m.viewport.SetContent(content)

		// Auto-scroll to keep cursor action item in view.
		// Action lines are at the end of content:
		//   cursor=0 → 4th from end, cursor=1 → 3rd, cursor=2 → 2nd, cursor=3 → last
		lineFromEnd := 4 - m.cursor
		cursorLine := totalLines - 1 - lineFromEnd // 0-indexed

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

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

		switch m.state {
		case stateConfirm:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					m = m.refreshViewport()
				}
			case "down", "j":
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
				case 0: // Commit
					logger.Debug("user selected: commit")
					m.state = stateCommitting
					return m, m.commitCmd()
				case 1: // Regenerate
					logger.Debug("user selected: regenerate")
					m.state = stateGenerating
					return m, m.generateCommitCmd()
				case 2: // Edit
					logger.Debug("user selected: edit")
					m.state = stateEditing
					m.textarea.SetValue(m.commitMsg)
					return m, textarea.Blink
				case 3: // Cancel
					logger.Info("user cancelled operation")
					m.quitting = true
					return m, tea.Quit
				}
			}

		case stateEditing:
			if msg.String() == "esc" {
				m.commitMsg = m.textarea.Value()
				m.state = stateConfirm
				m = m.refreshViewport()
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}

	case tea.MouseMsg:
		// Only handle mouse when viewport scroll is active.
		if m.state == stateConfirm && m.needsScroll && m.viewportReady {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(m.innerWidth() - 4)

		vpHeight := m.innerHeight()
		if !m.viewportReady {
			m.viewport = viewport.New(m.innerWidth(), vpHeight)
			m.viewportReady = true
		} else {
			m.viewport.Width = m.innerWidth()
			m.viewport.Height = vpHeight
		}
		m = m.refreshViewport()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case commitResultMsg:
		if msg.err != nil {
			logger.Error("commit generation failed", "error", msg.err)
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}
		logger.Debug("commit message received", "length", len(msg.content))
		m.commitMsg = msg.content
		m.state = stateConfirm
		m.cursor = 0
		m = m.refreshViewport()

	case commitDoneMsg:
		if msg.err != nil {
			logger.Error("commit operation failed", "error", msg.err)
			m.err = msg.err
		} else {
			logger.Info("commit operation completed successfully")
		}
		m.state = stateDone
		return m, tea.Quit
	}

	return m, nil
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	var inner string

	switch m.state {
	case stateGenerating:
		inner = fmt.Sprintf("\n %s Generating commit message...\n", m.spinner.View())

	case stateCommitting:
		inner = fmt.Sprintf("\n %s Committing...\n", m.spinner.View())

	case stateConfirm:
		if m.needsScroll && m.viewportReady {
			// Content overflows → viewport (content already set in Update via refreshViewport).
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
		} else {
			// Content fits — use cached content built in Update (zero allocations here).
			inner = m.cachedContent
		}

	case stateEditing:
		var b strings.Builder
		b.WriteString(styleEditTitle.Render("Edit Commit Message"))
		b.WriteString("\n")
		b.WriteString(m.textarea.View())
		b.WriteString("\n\n (Press Esc to finish editing)\n")
		inner = b.String()

	case stateDone:
		if m.err != nil {
			inner = fmt.Sprintf("\n ✗ Error: %v\n", m.err)
		} else {
			inner = "\n ✓ Committed successfully!\n"
		}
	}

	if inner == "" {
		return ""
	}

	// styleWindow is pre-computed; only add Width and conditional Height here.
	ws := styleWindow.Width(m.width - 2)
	if m.needsScroll && m.height > 0 {
		ws = ws.Height(m.height - 2)
	}

	return ws.Render(inner)
}