package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

func TestCommitCmd_GitPath(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	// stage a change so there is something to commit
	writeFile(t, dir, "feature.txt", "new feature\n")
	runGit(t, dir, "add", ".")

	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), dir, fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
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
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeProvider{err: errors.New("boom")}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
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
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
	m.state = stateCopied
	u, _ := m.Update(copyDoneMsg{})
	tm := u.(tuiModel)
	if tm.state != stateConfirm {
		t.Errorf("copyDoneMsg should return to confirm, got %v", tm.state)
	}
}

func TestTui_CommitDoneError(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
	u, cmd := m.Update(commitDoneMsg{err: errors.New("commit failed")})
	tm := u.(tuiModel)
	if tm.state != stateDone || tm.err == nil || cmd == nil {
		t.Errorf("commit error should move to done with quit; state=%v err=%v", tm.state, tm.err)
	}
}

func TestTui_EnterRegenerate(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeProvider{resp: "feat: regen"}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
	u, _ := m.Update(commitResultMsg{content: "first"})
	tm := u.(tuiModel)
	tm.cursor = 1 // Regenerate
	u, cmd := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	if tm.state != stateRegenHint || cmd == nil {
		t.Fatalf("Regenerate should prompt for guidance; state=%v", tm.state)
	}
	// type a hint then confirm
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shorter")})
	tm = u.(tuiModel)
	u, cmd = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	if tm.state != stateGenerating || cmd == nil {
		t.Errorf("after guidance, should regenerate; state=%v", tm.state)
	}
	if tm.regenHint != "shorter" {
		t.Errorf("regenHint = %q; want 'shorter'", tm.regenHint)
	}
}

func TestTui_RegenHintCancel(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeProvider{resp: "x"}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
	u, _ := m.Update(commitResultMsg{content: "msg"})
	tm := u.(tuiModel)
	tm.cursor = 1
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = u.(tuiModel)
	// esc cancels guidance back to confirm
	u, _ = tm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm = u.(tuiModel)
	if tm.state != stateConfirm {
		t.Errorf("esc should return to confirm, got %v", tm.state)
	}
}

func TestTui_PgUpPgDownWhenScrolling(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, "", tr, nil)
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
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(), fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, false, tr)
	m.state = reviewStateCopied
	u, _ := m.Update(reviewCopyDoneMsg{})
	rm := u.(reviewModel)
	if rm.state != reviewStateDone {
		t.Errorf("reviewCopyDoneMsg should return to done, got %v", rm.state)
	}
}

func TestReview_FullRegenerate(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(), fakeProvider{resp: "## x"}, baseMsgs(), 0.7, 5*time.Second, false, tr)
	u, _ := m.Update(reviewResultMsg{content: "## Code Quality\nok"})
	rm := u.(reviewModel)
	rm.cursor = 1 // Regenerate
	u, cmd := rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if rm.state != reviewStateRegenHint || cmd == nil {
		t.Errorf("Regenerate should open guidance hint; state=%v", rm.state)
	}
	u, cmd = rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if rm.state != reviewStateAnalyzing || cmd == nil {
		t.Errorf("confirming hint should re-analyze; state=%v", rm.state)
	}
}

func TestReview_QuickRegenerateStaysQuick(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(), fakeProvider{resp: "## c"}, baseMsgs(), 0.7, 5*time.Second, true, tr)
	u, _ := m.Update(reviewResultMsg{content: "## Conclusion\nok"})
	rm := u.(reviewModel)
	rm.cursor = 2 // Regenerate (quick)
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if !rm.isQuickMode || rm.state != reviewStateRegenHint {
		t.Errorf("quick regenerate should stay quick + hint; quick=%v state=%v", rm.isQuickMode, rm.state)
	}
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if !rm.isQuickMode || rm.state != reviewStateAnalyzing {
		t.Errorf("quick regenerate confirm should stay quick + analyzing; quick=%v state=%v", rm.isQuickMode, rm.state)
	}
}

func TestReview_QuickExitQuits(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(), fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true, tr)
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
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(), fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true, tr)
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	rm := u.(reviewModel)
	if !rm.quitting || cmd == nil {
		t.Error("ctrl+c should quit review")
	}
}
