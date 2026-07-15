package app

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

func TestFormatReviewText(t *testing.T) {
	in := strings.Join([]string{
		"```markdown",
		"## Code Quality",
		"### Details",
		"- a bullet",
		"1. numbered",
		"> a quote",
		"---",
		"plain **bold** and `code`",
		"```",
	}, "\n")
	out := formatReviewText(in)
	// code fences stripped
	if strings.Contains(out, "```") {
		t.Error("code fences should be stripped")
	}
	// bullet converted
	if !strings.Contains(out, "•") {
		t.Error("bullet not converted")
	}
	// content preserved
	for _, want := range []string{"Code Quality", "Details", "numbered", "quote", "bold", "code"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatReviewText missing %q", want)
		}
	}
}

func TestApplyInlineStyles(t *testing.T) {
	out := applyInlineStyles("use `go test` and **bold** text")
	if !strings.Contains(out, "go test") || !strings.Contains(out, "bold") {
		t.Errorf("inline styles dropped content: %q", out)
	}
}

func newTestReviewModel(quick bool) reviewModel {
	tr := i18n.New(i18n.LocaleEN)
	return newReviewModel(fakeProvider{resp: "## Conclusion\nok"}, baseMsgs(), 0.7, 5*time.Second, quick, tr)
}

func TestReview_ResultQuickDone(t *testing.T) {
	m := newTestReviewModel(true)
	u, _ := m.Update(reviewResultMsg{content: "## Conclusion\nlooks good"})
	rm := u.(reviewModel)
	if rm.state != reviewStateQuickDone {
		t.Errorf("expected quick done, got %v", rm.state)
	}
	if rm.report != "## Conclusion\nlooks good" {
		t.Errorf("report = %q", rm.report)
	}
}

func TestReview_ResultFullDone(t *testing.T) {
	m := newTestReviewModel(false)
	u, _ := m.Update(reviewResultMsg{content: "## Code Quality\nfine"})
	rm := u.(reviewModel)
	if rm.state != reviewStateDone {
		t.Errorf("expected full done, got %v", rm.state)
	}
}

func TestReview_ResultError(t *testing.T) {
	m := newTestReviewModel(true)
	u, _ := m.Update(reviewResultMsg{err: os.ErrDeadlineExceeded})
	rm := u.(reviewModel)
	if rm.err == nil {
		t.Error("expected error stored")
	}
}

func TestReview_QuickViewDetailsSwitchesToFull(t *testing.T) {
	m := newTestReviewModel(true)
	u, _ := m.Update(reviewResultMsg{content: "quick"})
	rm := u.(reviewModel)
	rm.cursor = 0 // View Details
	u, cmd := rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if rm.isQuickMode {
		t.Error("View Details should disable quick mode")
	}
	if rm.state != reviewStateAnalyzing {
		t.Errorf("expected re-analyzing, got %v", rm.state)
	}
	if cmd == nil {
		t.Error("expected regenerate command")
	}
}

func TestReview_QuickSuggestSetsSwitchFlag(t *testing.T) {
	m := newTestReviewModel(true)
	u, _ := m.Update(reviewResultMsg{content: "quick"})
	rm := u.(reviewModel)
	rm.cursor = 1 // Suggest Commit Message
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if !rm.switchToSuggest || !rm.quitting {
		t.Errorf("expected switchToSuggest+quitting, got %v/%v", rm.switchToSuggest, rm.quitting)
	}
}

func TestReview_FullExitQuits(t *testing.T) {
	m := newTestReviewModel(false)
	u, _ := m.Update(reviewResultMsg{content: "full"})
	rm := u.(reviewModel)
	rm.cursor = 2 // Exit
	u, cmd := rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if !rm.quitting || cmd == nil {
		t.Error("Exit should quit")
	}
}

func TestReview_NavigationClampQuick(t *testing.T) {
	m := newTestReviewModel(true)
	u, _ := m.Update(reviewResultMsg{content: "quick"})
	rm := u.(reviewModel)
	// up at 0 clamps
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyUp})
	rm = u.(reviewModel)
	if rm.cursor != 0 {
		t.Errorf("cursor should clamp at 0, got %d", rm.cursor)
	}
	// down 5 times clamps at 3 (quick has 4 options)
	for i := 0; i < 5; i++ {
		u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyDown})
		rm = u.(reviewModel)
	}
	if rm.cursor != 3 {
		t.Errorf("cursor should clamp at 3, got %d", rm.cursor)
	}
}

func TestReview_GenerateReviewCmd(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(fakeProvider{resp: "```markdown\n## Conclusion\nok\n```"}, baseMsgs(), 0.7, 5*time.Second, true, tr)
	msg := m.generateReviewCmd()()
	res, ok := msg.(reviewResultMsg)
	if !ok {
		t.Fatalf("expected reviewResultMsg, got %T", msg)
	}
	if res.err != nil || !strings.Contains(res.content, "Conclusion") {
		t.Errorf("unexpected result: content=%q err=%v", res.content, res.err)
	}
}

func TestReview_WindowSizeAndView(t *testing.T) {
	m := newTestReviewModel(false)
	u, _ := m.Update(reviewResultMsg{content: "## Code Quality\nlooks fine"})
	rm := u.(reviewModel)
	u, _ = rm.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	rm = u.(reviewModel)
	view := rm.View()
	if view == "" {
		t.Error("view should render content")
	}
}
