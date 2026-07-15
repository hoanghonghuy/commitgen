package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

const teatestWait = 5 * time.Second

func waitForText(t *testing.T, tm *teatest.TestModel, substr string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte(substr))
	}, teatest.WithDuration(teatestWait), teatest.WithCheckInterval(20*time.Millisecond))
}

func TestTeatest_SuggestCancel(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel("/repo", fakeProvider{resp: "```text\nfeat: add cancel\n```"}, baseMsgs(), 0.7, 2*time.Second, false, "", tr)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "feat: add cancel")

	// Navigate to "Cancel" (index 3) and confirm.
	for i := 0; i < 3; i++ {
		tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	}
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(teatestWait))
	fm := tm.FinalModel(t).(tuiModel)
	if !fm.quitting {
		t.Error("expected model to be quitting after Cancel")
	}
}

func TestTeatest_SuggestCommitViaHook(t *testing.T) {
	hookFile := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel("/repo", fakeProvider{resp: "```text\nfeat: hooked commit\n```"}, baseMsgs(), 0.7, 2*time.Second, false, hookFile, tr)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "feat: hooked commit")

	// Cursor starts at 0 = "Commit (Apply)".
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	waitForText(t, tm, "Committed successfully")
	tm.WaitFinished(t, teatest.WithFinalTimeout(teatestWait))

	b, err := os.ReadFile(hookFile)
	if err != nil {
		t.Fatalf("hook file not written: %v", err)
	}
	if string(b) != "feat: hooked commit" {
		t.Errorf("hook file content = %q", string(b))
	}
}

func TestTeatest_SuggestGenerateError(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel("/repo", fakeProvider{err: errors.New("provider offline")}, baseMsgs(), 0.7, 2*time.Second, false, "", tr)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.WaitFinished(t, teatest.WithFinalTimeout(teatestWait))
	fm := tm.FinalModel(t).(tuiModel)
	if fm.err == nil {
		t.Error("expected error stored on model after generation failure")
	}
}

func TestTeatest_ReviewQuickExit(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(fakeProvider{resp: "```markdown\n## Conclusion\nlooks good\n```"}, baseMsgs(), 0.7, 2*time.Second, true, tr)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "Quick Scan Result")

	// Navigate to "Exit" (index 3 in quick mode).
	for i := 0; i < 3; i++ {
		tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	}
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(teatestWait))
	fm := tm.FinalModel(t).(reviewModel)
	if !fm.quitting || fm.switchToSuggest {
		t.Errorf("expected plain quit; quitting=%v switchToSuggest=%v", fm.quitting, fm.switchToSuggest)
	}
}

func TestTeatest_ReviewSwitchToSuggest(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(fakeProvider{resp: "```markdown\n## Conclusion\nok\n```"}, baseMsgs(), 0.7, 2*time.Second, true, tr)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "Quick Scan Result")

	// "Suggest Commit Message" is index 1 in quick mode.
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(teatestWait))
	fm := tm.FinalModel(t).(reviewModel)
	if !fm.switchToSuggest {
		t.Error("expected switchToSuggest=true after selecting Suggest Commit Message")
	}
}

func TestTeatest_ReviewQuickViewDetails(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(fakeProvider{resp: "```markdown\n## Conclusion\nok\n```"}, baseMsgs(), 0.7, 2*time.Second, true, tr)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	waitForText(t, tm, "Quick Scan Result")

	// "View Details" is index 0 → switches to full review and re-runs.
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForText(t, tm, "Review Report")

	// Then Exit (full mode: Suggest=0, Regenerate=1, Exit=2).
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(teatestWait))
	fm := tm.FinalModel(t).(reviewModel)
	if fm.isQuickMode {
		t.Error("expected full (non-quick) mode after View Details")
	}
}
