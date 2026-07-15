package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func TestDumpPrompt_ToFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "prompt.json")
	msgs := vscodeprompt.BuildVSCodeMessages(vscodeprompt.Data{
		RepositoryName: "r",
		Changes:        []vscodeprompt.Change{{Path: "a.go", Diff: "d"}},
	})
	if err := dumpPrompt(msgs, out); err != nil {
		t.Fatalf("dumpPrompt error: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("output not written: %v", err)
	}
	var decoded []vscodeprompt.VSCodeMessage
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("expected 2 messages in dump, got %d", len(decoded))
	}
}

func TestDumpPrompt_ToStdout(t *testing.T) {
	// Redirect stdout to a pipe to capture output.
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	msgs := vscodeprompt.BuildVSCodeMessages(vscodeprompt.Data{RepositoryName: "r", Changes: []vscodeprompt.Change{{Path: "a.go", Diff: "d"}}})
	err := dumpPrompt(msgs, "")
	w.Close()
	if err != nil {
		t.Fatalf("dumpPrompt error: %v", err)
	}
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if n == 0 {
		t.Error("expected JSON written to stdout")
	}
}

func TestTui_ViewStates(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(),"/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true, "", tr, nil)

	m.state = stateGenerating
	if m.View() == "" {
		t.Error("generating view empty")
	}
	m.state = stateCommitting
	if m.View() == "" {
		t.Error("committing view empty")
	}
	m.state = stateEditing
	m.textarea.SetValue("edit me")
	if m.View() == "" {
		t.Error("editing view empty")
	}
	m.state = stateDone
	m.err = os.ErrPermission
	if m.View() == "" {
		t.Error("done(error) view empty")
	}
	m.err = nil
	if m.View() == "" {
		t.Error("done(success) view empty")
	}
	m.state = stateCopied
	if m.View() == "" {
		t.Error("copied view empty")
	}

	// quitting renders nothing
	m.quitting = true
	if m.View() != "" {
		t.Error("quitting view should be empty")
	}
}

func TestTui_RefreshViewportScroll(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(),"/repo", fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true, "", tr, nil)
	// small terminal forces scrolling
	u, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	m = u.(tuiModel)
	// long multi-line commit message
	long := ""
	for i := 0; i < 40; i++ {
		long += "line of commit body text\n"
	}
	u, _ = m.Update(commitResultMsg{content: long})
	m = u.(tuiModel)
	if !m.needsScroll {
		t.Error("expected needsScroll for tall content in short terminal")
	}
	if m.View() == "" {
		t.Error("scrolled view should render")
	}
}

func TestReview_ViewAnalyzingAndCopied(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(),fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true, tr)
	m.state = reviewStateAnalyzing
	if m.View() == "" {
		t.Error("analyzing view empty")
	}
	m.state = reviewStateCopied
	if m.View() == "" {
		t.Error("copied view empty")
	}
	m.quitting = true
	if m.View() != "" {
		t.Error("quitting view should be empty")
	}
}
