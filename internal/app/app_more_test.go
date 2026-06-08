package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTuiInit(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{resp: "x"}, baseMsgs(), 0.7, time.Second, false, "")
	if m.Init() == nil {
		t.Error("tuiModel.Init should return a command batch")
	}
}

func TestReviewInit(t *testing.T) {
	m := newReviewModel(fakeProvider{resp: "x"}, baseMsgs(), 0.7, time.Second, true)
	if m.Init() == nil {
		t.Error("reviewModel.Init should return a command batch")
	}
}

func TestGenerateCommitCmd_RawFallback(t *testing.T) {
	// Provider returns plain prose without a code block → raw text is used.
	m := newTuiModel("/repo", fakeProvider{resp: "just a plain message"}, baseMsgs(), 0.7, time.Second, false, "")
	msg := m.generateCommitCmd()()
	res := msg.(commitResultMsg)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if res.content != "just a plain message" {
		t.Errorf("expected raw fallback, got %q", res.content)
	}
}

func TestGenerateReviewCmd_Error(t *testing.T) {
	m := newReviewModel(fakeProvider{err: errors.New("net down")}, baseMsgs(), 0.7, time.Second, true)
	msg := m.generateReviewCmd()()
	res := msg.(reviewResultMsg)
	if res.err == nil {
		t.Error("expected error from provider")
	}
}

func TestBuildPromptData_TruncatesLargeNewFile(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# init\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	// New file > 100KB, staged but not in HEAD → triggers diff truncation,
	// the "orig empty -> read working tree" branch, and content truncation.
	big := strings.Repeat("abcdefghij\n", 12000) // ~120KB
	writeFile(t, dir, "big.txt", big)
	runGit(t, dir, "add", ".")

	data, err := buildPromptData(ctx, dir, 5, 10, false, "", nil)
	if err != nil {
		t.Fatalf("buildPromptData error: %v", err)
	}
	if len(data.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(data.Changes))
	}
	if !strings.Contains(data.Changes[0].Diff, "truncated") {
		t.Errorf("expected truncated diff marker, got len=%d", len(data.Changes[0].Diff))
	}
}

func TestTui_EditingTypingUpdatesTextarea(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{}, baseMsgs(), 0.7, time.Second, false, "")
	u, _ := m.Update(commitResultMsg{content: "msg"})
	tm := u.(tuiModel)
	tm.cursor = 2 // Edit
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	if tm.state != stateEditing {
		t.Fatalf("expected editing state")
	}
	// type a character → textarea handles it without error
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	tm = u.(tuiModel)
	if tm.state != stateEditing {
		t.Errorf("should remain in editing after typing, got %v", tm.state)
	}
}

func TestReview_ViewErrorBranches(t *testing.T) {
	// full done with error
	m := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, time.Second, false)
	m.state = reviewStateDone
	m.err = errors.New("boom")
	if !strings.Contains(m.View(), "boom") {
		t.Error("done error view should contain error text")
	}

	// quick done with error
	m2 := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, time.Second, true)
	m2.state = reviewStateQuickDone
	m2.err = errors.New("kaput")
	if !strings.Contains(m2.View(), "kaput") {
		t.Error("quick done error view should contain error text")
	}
}

func TestReview_PgUpPgDown(t *testing.T) {
	m := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, time.Second, false)
	u, _ := m.Update(reviewResultMsg{content: strings.Repeat("## Section\nbody text here\n", 40)})
	rm := u.(reviewModel)
	u, _ = rm.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	rm = u.(reviewModel)
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	rm = u.(reviewModel)
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	rm = u.(reviewModel)
	if rm.state != reviewStateDone {
		t.Errorf("state should remain done, got %v", rm.state)
	}
}

func TestReview_MouseScroll(t *testing.T) {
	m := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, time.Second, false)
	u, _ := m.Update(reviewResultMsg{content: strings.Repeat("line\n", 40)})
	rm := u.(reviewModel)
	u, _ = rm.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	rm = u.(reviewModel)
	// mouse wheel event should be forwarded to viewport without panicking
	u, _ = rm.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	rm = u.(reviewModel)
	if rm.state != reviewStateDone {
		t.Errorf("state should remain done after mouse, got %v", rm.state)
	}
}
