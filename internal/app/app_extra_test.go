package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
)

func TestCommitCmd_GitPath(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	// stage a change so there is something to commit
	writeFile(t, dir, "feature.txt", "new feature\n")
	runGit(t, dir, "add", ".")

	m := newTuiModel(dir, fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "")
	m.commitMsg = "feat: add feature file"

	msg := m.commitCmd()()
	done, ok := msg.(commitDoneMsg)
	if !ok {
		t.Fatalf("expected commitDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("git commit failed: %v", done.err)
	}

	commits, _ := gitx.RecentCommits(context.Background(), dir, 1)
	if len(commits) != 1 || commits[0] != "feat: add feature file" {
		t.Errorf("commit not recorded: %v", commits)
	}
}

func TestGenerateCommitCmd_Error(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{err: errors.New("boom")}, baseMsgs(), 0.7, 5*time.Second, false, "")
	msg := m.generateCommitCmd()()
	res, ok := msg.(commitResultMsg)
	if !ok {
		t.Fatalf("expected commitResultMsg, got %T", msg)
	}
	if res.err == nil {
		t.Error("expected error propagated from provider")
	}
}

func TestTui_CopyDoneReturnsToConfirm(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "")
	m.state = stateCopied
	u, _ := m.Update(copyDoneMsg{})
	tm := u.(tuiModel)
	if tm.state != stateConfirm {
		t.Errorf("copyDoneMsg should return to confirm, got %v", tm.state)
	}
}

func TestTui_CommitDoneError(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "")
	u, cmd := m.Update(commitDoneMsg{err: errors.New("commit failed")})
	tm := u.(tuiModel)
	if tm.state != stateDone || tm.err == nil || cmd == nil {
		t.Errorf("commit error should move to done with quit; state=%v err=%v", tm.state, tm.err)
	}
}

func TestTui_EnterRegenerate(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{resp: "feat: regen"}, baseMsgs(), 0.7, 5*time.Second, false, "")
	u, _ := m.Update(commitResultMsg{content: "first"})
	tm := u.(tuiModel)
	tm.cursor = 1 // Regenerate
	u, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	if tm.state != stateGenerating || cmd == nil {
		t.Errorf("Regenerate should re-enter generating state with cmd; state=%v", tm.state)
	}
}

func TestTui_PgUpPgDownWhenScrolling(t *testing.T) {
	m := newTuiModel("/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "")
	u, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	m = u.(tuiModel)
	long := strings.Repeat("body line of text\n", 50)
	u, _ = m.Update(commitResultMsg{content: long})
	m = u.(tuiModel)
	if !m.needsScroll {
		t.Fatal("setup expected scrolling content")
	}
	// pgdown then pgup should not panic and keep state confirm
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = u.(tuiModel)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = u.(tuiModel)
	if m.state != stateConfirm {
		t.Errorf("state should remain confirm, got %v", m.state)
	}
}

func TestReview_CopyDoneReturnsToDone(t *testing.T) {
	m := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false)
	m.state = reviewStateCopied
	u, _ := m.Update(reviewCopyDoneMsg{})
	rm := u.(reviewModel)
	if rm.state != reviewStateDone {
		t.Errorf("reviewCopyDoneMsg should return to done, got %v", rm.state)
	}
}

func TestReview_FullRegenerate(t *testing.T) {
	m := newReviewModel(fakeProvider{resp: "## x"}, baseMsgs(), 0.7, 5*time.Second, false)
	u, _ := m.Update(reviewResultMsg{content: "## Code Quality\nok"})
	rm := u.(reviewModel)
	rm.cursor = 1 // Regenerate
	u, cmd := rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if rm.state != reviewStateAnalyzing || cmd == nil {
		t.Errorf("Regenerate should re-analyze with cmd; state=%v", rm.state)
	}
}

func TestReview_QuickRegenerateStaysQuick(t *testing.T) {
	m := newReviewModel(fakeProvider{resp: "## c"}, baseMsgs(), 0.7, 5*time.Second, true)
	u, _ := m.Update(reviewResultMsg{content: "## Conclusion\nok"})
	rm := u.(reviewModel)
	rm.cursor = 2 // Regenerate (quick)
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if !rm.isQuickMode || rm.state != reviewStateAnalyzing {
		t.Errorf("quick regenerate should stay quick + analyzing; quick=%v state=%v", rm.isQuickMode, rm.state)
	}
}

func TestReview_QuickExitQuits(t *testing.T) {
	m := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true)
	u, _ := m.Update(reviewResultMsg{content: "## Conclusion\nok"})
	rm := u.(reviewModel)
	rm.cursor = 3 // Exit
	u, cmd := rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if !rm.quitting || cmd == nil {
		t.Error("quick Exit should quit")
	}
}

func TestReview_CtrlCQuits(t *testing.T) {
	m := newReviewModel(fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true)
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	rm := u.(reviewModel)
	if !rm.quitting || cmd == nil {
		t.Error("ctrl+c should quit review")
	}
}
