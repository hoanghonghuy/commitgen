package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func baseMsgs() []vscodeprompt.VSCodeMessage {
	return []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleSystem, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "s"}}},
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "u"}}},
	}
}

func TestCalcInnerWidthHeight(t *testing.T) {
	if got := calcInnerWidth(100); got != 96 {
		t.Errorf("calcInnerWidth(100) = %d", got)
	}
	if got := calcInnerWidth(5); got != 10 { // min clamp
		t.Errorf("calcInnerWidth min = %d", got)
	}
	if got := calcInnerHeight(40); got != 38 {
		t.Errorf("calcInnerHeight(40) = %d", got)
	}
	if got := calcInnerHeight(1); got != 3 { // min clamp
		t.Errorf("calcInnerHeight min = %d", got)
	}
}

func TestCountLines(t *testing.T) {
	if got := countLines(""); got != 0 {
		t.Errorf("empty = %d", got)
	}
	if got := countLines("one"); got != 1 {
		t.Errorf("one line = %d", got)
	}
	if got := countLines("a\nb\nc"); got != 3 {
		t.Errorf("three lines = %d", got)
	}
}

func TestScrollHintText(t *testing.T) {
	if !strings.Contains(scrollHintText(0, true, false), "PgDn") {
		t.Error("top hint should mention PgDn")
	}
	if !strings.Contains(scrollHintText(100, false, true), "PgUp") {
		t.Error("bottom hint should mention PgUp")
	}
	mid := scrollHintText(50, false, false)
	if !strings.Contains(mid, "50%") {
		t.Errorf("mid hint should contain percent: %q", mid)
	}
}

func newTestModel() tuiModel {
	return newTuiModel("/repo", fakeProvider{resp: "feat: x"}, baseMsgs(), 0.7, 5*time.Second, true, "")
}

func TestTui_CommitResultMovesToConfirm(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(commitResultMsg{content: "feat: add feature"})
	tm := updated.(tuiModel)
	if tm.state != stateConfirm {
		t.Errorf("expected stateConfirm, got %v", tm.state)
	}
	if tm.commitMsg != "feat: add feature" {
		t.Errorf("commitMsg = %q", tm.commitMsg)
	}
}

func TestTui_CommitResultErrorMovesToDone(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(commitResultMsg{err: os.ErrDeadlineExceeded})
	tm := updated.(tuiModel)
	if tm.state != stateDone || tm.err == nil {
		t.Errorf("expected stateDone with error, got state=%v err=%v", tm.state, tm.err)
	}
}

func TestTui_NavigationAndCursorBounds(t *testing.T) {
	m := newTestModel()
	u, _ := m.Update(commitResultMsg{content: "msg"})
	tm := u.(tuiModel)

	// move down twice
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm = u.(tuiModel)
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm = u.(tuiModel)
	if tm.cursor != 2 {
		t.Errorf("cursor after 2 downs = %d; want 2", tm.cursor)
	}
	// up once
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyUp})
	tm = u.(tuiModel)
	if tm.cursor != 1 {
		t.Errorf("cursor after up = %d; want 1", tm.cursor)
	}
	// cannot go below 0
	tm.cursor = 0
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyUp})
	tm = u.(tuiModel)
	if tm.cursor != 0 {
		t.Errorf("cursor must clamp at 0, got %d", tm.cursor)
	}
	// cannot exceed max
	tm.cursor = confirmActionCount - 1
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm = u.(tuiModel)
	if tm.cursor != confirmActionCount-1 {
		t.Errorf("cursor must clamp at max, got %d", tm.cursor)
	}
}

func TestTui_EnterCancelQuits(t *testing.T) {
	m := newTestModel()
	u, _ := m.Update(commitResultMsg{content: "msg"})
	tm := u.(tuiModel)
	tm.cursor = 3 // Cancel
	u, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	if !tm.quitting {
		t.Error("Cancel should set quitting")
	}
	if cmd == nil {
		t.Error("Cancel should return quit cmd")
	}
}

func TestTui_EnterEditEntersEditing(t *testing.T) {
	m := newTestModel()
	u, _ := m.Update(commitResultMsg{content: "the message"})
	tm := u.(tuiModel)
	tm.cursor = 2 // Edit
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	if tm.state != stateEditing {
		t.Errorf("expected stateEditing, got %v", tm.state)
	}
	if tm.textarea.Value() != "the message" {
		t.Errorf("textarea should be seeded with commitMsg, got %q", tm.textarea.Value())
	}
	// esc returns to confirm and saves edited value
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm = u.(tuiModel)
	if tm.state != stateConfirm {
		t.Errorf("esc should return to confirm, got %v", tm.state)
	}
}

func TestTui_CtrlCQuits(t *testing.T) {
	m := newTestModel()
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm := u.(tuiModel)
	if !tm.quitting || cmd == nil {
		t.Error("ctrl+c should quit")
	}
}

func TestTui_CommitCmdWritesHookFile(t *testing.T) {
	dir := t.TempDir()
	hookFile := filepath.Join(dir, "COMMIT_EDITMSG")
	m := newTuiModel("/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, hookFile)
	m.commitMsg = "feat: via hook"

	msg := m.commitCmd()()
	done, ok := msg.(commitDoneMsg)
	if !ok {
		t.Fatalf("expected commitDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("commitCmd error: %v", done.err)
	}
	b, err := os.ReadFile(hookFile)
	if err != nil {
		t.Fatalf("hook file not written: %v", err)
	}
	if string(b) != "feat: via hook" {
		t.Errorf("hook file content = %q", string(b))
	}
}

func TestTui_GenerateCommitCmd(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{resp: "```text\nfeat: gen\n```"}, baseMsgs(), 0.7, 5*time.Second, true, "")
	msg := m.generateCommitCmd()()
	res, ok := msg.(commitResultMsg)
	if !ok {
		t.Fatalf("expected commitResultMsg, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if res.content != "feat: gen" {
		t.Errorf("content = %q; want extracted code block", res.content)
	}
}

func TestTui_WindowSizeAndView(t *testing.T) {
	m := newTestModel()
	u, _ := m.Update(commitResultMsg{content: "feat: something"})
	tm := u.(tuiModel)
	u, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	tm = u.(tuiModel)
	if tm.width != 100 || tm.height != 40 {
		t.Errorf("size not stored: %dx%d", tm.width, tm.height)
	}
	view := tm.View()
	if !strings.Contains(view, "feat: something") {
		t.Errorf("view should contain commit message")
	}
}
