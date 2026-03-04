package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

type tuiState int

const (
	stateGenerating tuiState = iota
	stateConfirm
	stateEditing
	stateDone
)

type tuiModel struct {
	state tuiState
	width int

	// Dependencies
	provider     ai.Provider
	initialMsgs  []vscodeprompt.VSCodeMessage
	temp         float64
	timeout      time.Duration
	conventional bool
	hookFile     string
	repoRoot     string

	// Components
	spinner  spinner.Model
	textarea textarea.Model

	// Data
	commitMsg string
	cursor    int
	err       error
	quitting  bool
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
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

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
		currentMsgs := make([]vscodeprompt.VSCodeMessage, len(m.initialMsgs))
		copy(currentMsgs, m.initialMsgs)

		if m.conventional {
			reminderMsg := vscodeprompt.VSCodeMessage{
				Role: 1, // user
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
			return commitResultMsg{err: err}
		}

		msg, ok := vscodeprompt.ExtractOneTextCodeBlock(raw)
		if !ok {
			msg = raw
		}
		return commitResultMsg{content: msg}
	}
}

func (m tuiModel) commitCmd() tea.Cmd {
	return func() tea.Msg {
		if m.hookFile != "" {
			err := os.WriteFile(m.hookFile, []byte(m.commitMsg), 0644)
			return commitDoneMsg{err: err}
		}
		err := gitx.Commit(context.Background(), m.repoRoot, m.commitMsg)
		return commitDoneMsg{err: err}
	}
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
				}
			case "down", "j":
				if m.cursor < 3 {
					m.cursor++
				}
			case "enter":
				switch m.cursor {
				case 0: // Commit
					m.state = stateGenerating // Show "Committing..."
					return m, m.commitCmd()
				case 1: // Regenerate
					m.state = stateGenerating
					return m, m.generateCommitCmd()
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
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.textarea.SetWidth(msg.Width - 4)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case commitResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateDone
			return m, tea.Quit
		}
		m.commitMsg = msg.content
		m.state = stateConfirm
		m.cursor = 0

	case commitDoneMsg:
		if msg.err != nil {
			m.err = msg.err
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

	var b strings.Builder

	switch m.state {
	case stateGenerating:
		b.WriteString(fmt.Sprintf("\n %s Generating commit message...\n", m.spinner.View()))

	case stateConfirm:
		b.WriteString(m.renderCommitMessage())
		b.WriteString("\n Action\n")
		options := []string{"Commit (Apply)", "Regenerate", "Edit", "Cancel"}
		for i, opt := range options {
			cursor := "  "
			style := lipgloss.NewStyle()
			if m.cursor == i {
				cursor = "> "
				style = style.Foreground(lipgloss.Color("212")).Bold(true)
			}
			b.WriteString(fmt.Sprintf("┃ %s%s\n", cursor, style.Render(opt)))
		}

	case stateEditing:
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).MarginLeft(2).Render("Edit Commit Message"))
		b.WriteString("\n")
		b.WriteString(m.textarea.View())
		b.WriteString("\n\n (Press Esc to finish editing)\n")

	case stateDone:
		if m.err != nil {
			return fmt.Sprintf("\n Error: %v\n", m.err)
		}
	}

	return b.String()
}

func (m tuiModel) renderCommitMessage() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212")).
		Bold(true).
		MarginLeft(2)

	contentStyle := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("240")).
		PaddingLeft(1).
		Width(m.width - 4).
		MarginBottom(1)

	return fmt.Sprintf("\n%s\n%s\n", titleStyle.Render("Generated Commit Message"), contentStyle.Render(m.commitMsg))
}