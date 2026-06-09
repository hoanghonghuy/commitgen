package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
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
	stateGenerating tuiState = iota // AI is generating commit message
	stateCommitting                 // Performing git commit
	stateConfirm
	stateEditing
	stateRegenHint
	stateChoose
	stateDone
	stateCopied
)

const (
	confirmActionCount = 4 // number of options in confirm menu
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
	amend        bool
	count        int

	// Components
	spinner       spinner.Model
	textarea      textarea.Model
	hintInput     textinput.Model
	viewport      viewport.Model
	viewportReady bool
	needsScroll   bool // true when content exceeds inner height

	// Data
	commitMsg     string
	cachedContent string // built once in Update, read in View — avoids per-frame rebuild
	cursor        int
	regenHint     string         // optional user guidance for regeneration
	candidates    []string       // multiple generated candidates (when count > 1)
	streamView    string         // accumulated text while streaming
	streamCh      chan streamEvent // delta channel for streaming providers
	err           error
	quitting      bool
}

// streamEvent carries an incremental streaming update or the final result.
type streamEvent struct {
	delta string
	full  string
	err   error
	done  bool
}

type commitResultMsg struct {
	content string
	err     error
}

// candidatesMsg carries multiple generated candidate messages (count > 1).
type candidatesMsg struct {
	contents []string
	err      error
}

// copyDoneMsg is sent after clipboard copy feedback expires.
type copyDoneMsg struct{}

type commitDoneMsg struct {
	err error
}

func newTuiModel(repoRoot string, provider ai.Provider, msgs []vscodeprompt.VSCodeMessage, temp float64, timeout time.Duration, conventional bool, hookFile string) tuiModel {
	s := newSpinnerModel()

	ta := textarea.New()
	ta.Placeholder = "Enter commit message..."
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(5)

	// Initialize viewport with default size
	vp := newDefaultViewport(80, 20)

	hi := textinput.New()
	hi.Placeholder = "optional guidance: shorter, in Vietnamese, focus on why..."

	return tuiModel{
		state:         stateGenerating,
		provider:      provider,
		initialMsgs:   msgs,
		temp:          temp,
		timeout:       timeout,
		conventional:  conventional,
		hookFile:      hookFile,
		repoRoot:      repoRoot,
		spinner:       s,
		textarea:      ta,
		hintInput:     hi,
		viewport:      vp,
		viewportReady: true, // Mark as ready immediately
		width:         80,
		height:        24,
		streamCh:      make(chan streamEvent, 256),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.generateCmd())
}

// generateCmd dispatches to single or multi-candidate generation based on count.
func (m tuiModel) generateCmd() tea.Cmd {
	if m.count > 1 {
		return m.generateCandidatesCmd()
	}
	// Use streaming when the provider supports it (single-message path only).
	if sp, ok := m.provider.(ai.StreamProvider); ok && m.streamCh != nil {
		msgs := m.buildGenMessages()
		if m.conventional {
			msgs = append(msgs, conventionalReminder())
		}
		return tea.Batch(startStreamCmd(sp, m.streamCh, msgs, m.temp, m.timeout), waitStreamCmd(m.streamCh))
	}
	return m.generateCommitCmd()
}

// startStreamCmd launches the streaming generation in a goroutine, pushing
// deltas and a final event onto ch.
func startStreamCmd(sp ai.StreamProvider, ch chan streamEvent, msgs []vscodeprompt.VSCodeMessage, temp float64, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			full, err := sp.GenerateStream(ctx, msgs, clampTemperature(temp), func(d string) {
				ch <- streamEvent{delta: d}
			})
			ch <- streamEvent{done: true, full: full, err: err}
		}()
		return nil
	}
}

// waitStreamCmd blocks until the next streaming event is available.
func waitStreamCmd(ch chan streamEvent) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// buildGenMessages returns the prompt messages including optional regen guidance.
func (m tuiModel) buildGenMessages() []vscodeprompt.VSCodeMessage {
	if strings.TrimSpace(m.regenHint) == "" {
		return m.initialMsgs
	}
	return append(append([]vscodeprompt.VSCodeMessage{}, m.initialMsgs...), vscodeprompt.VSCodeMessage{
		Role:    vscodeprompt.RoleUser,
		Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "Additional guidance for the commit message: " + m.regenHint}},
	})
}

// generateCandidatesCmd generates m.count candidate messages sequentially.
func (m tuiModel) generateCandidatesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()

		msgs := m.buildGenMessages()
		out := make([]string, 0, m.count)
		for i := 0; i < m.count; i++ {
			msg, err := generateCommitMessage(ctx, m.provider, msgs, m.temp, m.conventional)
			if err != nil {
				logger.Error("failed to generate candidate", "error", err)
				return candidatesMsg{err: err}
			}
			out = append(out, msg)
		}
		return candidatesMsg{contents: out}
	}
}

func (m tuiModel) generateCommitCmd() tea.Cmd {
	return func() tea.Msg {
		msgs := m.buildGenMessages()

		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		defer cancel()

		msg, err := generateCommitMessage(ctx, m.provider, msgs, m.temp, m.conventional)
		if err != nil {
			logger.Error("failed to generate commit message", "error", err)
			return commitResultMsg{err: err}
		}
		return commitResultMsg{content: msg}
	}
}

func (m tuiModel) commitCmd() tea.Cmd {
	return func() tea.Msg {
		if m.hookFile != "" {
			err := os.WriteFile(m.hookFile, []byte(m.commitMsg), 0644)
			if err != nil {
				logger.Error("failed to write hook file", "error", err, "path", m.hookFile)
			}
			return commitDoneMsg{err: err}
		}
		err := gitx.Commit(context.Background(), m.repoRoot, m.commitMsg)
		if m.amend {
			err = gitx.CommitAmend(context.Background(), m.repoRoot, m.commitMsg)
		}
		if err != nil {
			logger.Error("git commit failed", "error", err)
		}
		return commitDoneMsg{err: err}
	}
}

// innerWidth returns usable width inside the outer border+padding.
func (m tuiModel) innerWidth() int {
	return calcInnerWidth(m.width)
}

// innerHeight returns usable height inside the outer border.
func (m tuiModel) innerHeight() int {
	return calcInnerHeight(m.height)
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
	if m.state != stateConfirm || m.commitMsg == "" {
		return m
	}
	content := m.buildConfirmContent()
	m.cachedContent = content
	totalLines := countLines(content)
	m.needsScroll = totalLines > m.innerHeight()

	if m.needsScroll && m.viewportReady {
		m.viewport.SetContent(content)

		// Auto-scroll to keep cursor action item in view.
		// Action lines are at the end of content:
		//   cursor=0 → last option, cursor=max → first action item
		lineFromEnd := confirmActionCount - m.cursor
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
			case "y", "Y":
				if m.commitMsg != "" {
					if cmd := clipboardCopyCmd(m.commitMsg, copyDoneMsg{}); cmd != nil {
						m.state = stateCopied
						return m, cmd
					}
				}
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					m = m.refreshViewport()
				}
			case "down", "j":
				if m.cursor < confirmActionCount-1 {
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
					m.state = stateCommitting
					return m, m.commitCmd()
				case 1: // Regenerate
					m.state = stateRegenHint
					m.hintInput.SetValue("")
					m.hintInput.Focus()
					return m, textinput.Blink
				case 2: // Edit
					m.state = stateEditing
					m.textarea.SetValue(m.commitMsg)
					return m, textarea.Blink
				case 3: // Cancel
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

		case stateRegenHint:
			switch msg.String() {
			case "esc":
				// cancel guidance, return to confirm
				m.state = stateConfirm
				m = m.refreshViewport()
				return m, nil
			case "enter":
				m.regenHint = strings.TrimSpace(m.hintInput.Value())
				m.state = stateGenerating
				m.streamView = ""
				return m, m.generateCmd()
			}
			var cmd tea.Cmd
			m.hintInput, cmd = m.hintInput.Update(msg)
			return m, cmd

		case stateChoose:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.candidates)-1 {
					m.cursor++
				}
			case "enter":
				if len(m.candidates) > 0 {
					m.commitMsg = m.candidates[m.cursor]
					m.cursor = 0
					m.state = stateConfirm
					m = m.refreshViewport()
				}
				return m, nil
			case "r", "R":
				m.state = stateGenerating
				return m, m.generateCmd()
			}
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

		// Viewport height must account for border (2), padding (2), and hint (1).
		// innerHeight() subtracts border (2), so we subtract 3 more (padding 2 + hint 1).
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

	case commitResultMsg:
		if msg.err != nil {
			logger.Error("commit generation failed", "error", msg.err)
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}
		m.commitMsg = msg.content
		m.state = stateConfirm
		m.cursor = 0
		m = m.refreshViewport()
		return m, nil

	case streamEvent:
		if msg.done {
			if msg.err != nil {
				logger.Error("stream generation failed", "error", msg.err)
				m.err = msg.err
				m.state = stateDone
				return m, tea.Quit
			}
			content := msg.full
			if extracted, ok := vscodeprompt.ExtractOneTextCodeBlock(content); ok {
				content = extracted
			}
			m.commitMsg = content
			m.streamView = ""
			m.state = stateConfirm
			m.cursor = 0
			m = m.refreshViewport()
			return m, nil
		}
		m.streamView += msg.delta
		return m, waitStreamCmd(m.streamCh)

	case candidatesMsg:
		if msg.err != nil {
			logger.Error("candidate generation failed", "error", msg.err)
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}
		m.candidates = msg.contents
		m.cursor = 0
		// A single candidate skips the chooser.
		if len(m.candidates) == 1 {
			m.commitMsg = m.candidates[0]
			m.state = stateConfirm
			m = m.refreshViewport()
			return m, nil
		}
		m.state = stateChoose
		return m, nil

	case commitDoneMsg:
		if msg.err != nil {
			logger.Error("commit operation failed", "error", msg.err)
			m.err = msg.err
		}
		m.state = stateDone
		return m, tea.Quit

	case copyDoneMsg:
		m.state = stateConfirm
		return m, nil
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
		if strings.TrimSpace(m.streamView) != "" {
			var b strings.Builder
			b.WriteString("\n")
			b.WriteString(styleMsgTitle.Render("Generating commit message…"))
			b.WriteString("\n")
			b.WriteString(msgContentStyle(m.innerWidth() - 6).Render(m.streamView))
			b.WriteString("\n")
			inner = b.String()
		} else {
			inner = fmt.Sprintf("\n %s Generating commit message...\n", m.spinner.View())
		}

	case stateCommitting:
		inner = fmt.Sprintf("\n %s Committing...\n", m.spinner.View())

	case stateConfirm:
		if m.needsScroll && m.viewportReady {
			// Content overflows → viewport (content already set in Update via refreshViewport).
			pct := int(m.viewport.ScrollPercent() * 100)
			hint := scrollHintText(pct, m.viewport.AtTop(), m.viewport.AtBottom())
			inner = m.viewport.View() + "\n" + styleHint.Render(hint)
		} else {
			// Content fits — use cached content built in Update (zero allocations here).
			if m.cachedContent == "" {
				// Fallback: build content if not cached yet
				inner = m.buildConfirmContent()
			} else {
				inner = m.cachedContent
			}
		}

	case stateEditing:
		var b strings.Builder
		b.WriteString(styleEditTitle.Render("Edit Commit Message"))
		b.WriteString("\n")
		b.WriteString(m.textarea.View())
		b.WriteString("\n\n (Press Esc to finish editing)\n")
		inner = b.String()

	case stateRegenHint:
		var b strings.Builder
		b.WriteString(styleEditTitle.Render("Regenerate — Optional Guidance"))
		b.WriteString("\n")
		b.WriteString(m.hintInput.View())
		b.WriteString("\n\n (Enter to regenerate, Esc to cancel)\n")
		inner = b.String()

	case stateChoose:
		var b strings.Builder
		b.WriteString("\n")
		b.WriteString(styleMsgTitle.Render("Choose a Commit Message"))
		b.WriteString("\n")
		barStr := styleBar.Render("┃")
		for i, c := range m.candidates {
			line := strings.SplitN(c, "\n", 2)[0] // show first line of each candidate
			if m.cursor == i {
				b.WriteString(fmt.Sprintf("%s > %s\n", barStr, styleSelected.Render(line)))
			} else {
				b.WriteString(fmt.Sprintf("%s   %s\n", barStr, line))
			}
		}
		b.WriteString("\n")
		b.WriteString(styleHint.Render(" ↑↓ select  •  Enter choose  •  r regenerate all "))
		b.WriteString("\n")
		inner = b.String()

	case stateDone:
		if m.err != nil {
			inner = fmt.Sprintf("\n ✗ Error: %v\n", m.err)
		} else {
			inner = "\n ✓ Committed successfully!\n"
		}

	case stateCopied:
		inner = fmt.Sprintf("\n  ✓ Copied to clipboard!\n")
	}

	if inner == "" {
		return ""
	}

	// styleWindow is pre-computed; only add Width here.
	// DO NOT set Height — viewport already has correct height from WindowSizeMsg.
	// Setting Height on styleWindow causes lipgloss to crop content from the top,
	// which clips the border's top edge when viewport content exceeds the height.
	ws := styleWindow.Width(m.width - 2)

	return ws.Render(inner)
}
