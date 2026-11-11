package app

import (
	"context"
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
	return newReviewModel(context.Background(), fakeProvider{resp: "## Conclusion\nok"}, baseMsgs(), 0.7, 5*time.Second, quick, tr)
}

func TestReview_NavigationKeepsSelectedActionVisible(t *testing.T) {
	for _, quick := range []bool{false, true} {
		m := newTestReviewModel(quick)
		u, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = u.(reviewModel)
		u, _ = m.Update(reviewResultMsg{content: strings.Repeat("Review detail\n", 80)})
		m = u.(reviewModel)
		m.viewport.GotoBottom()
		for _, key := range []tea.KeyType{tea.KeyDown, tea.KeyUp} {
			u, _ = m.Update(tea.KeyMsg{Type: key})
			m = u.(reviewModel)
			if !strings.Contains(m.View(), "Suggest Commit Message") {
				t.Errorf("quick=%v: action hidden after navigation; scroll=%d", quick, m.viewport.YOffset)
			}
		}
	}
}

func TestReview_CancelRegenerationRestoresReport(t *testing.T) {
	for _, quick := range []bool{false, true} {
		m := newTestReviewModel(quick)
		const original = "Original useful report"
		u, _ := m.Update(reviewResultMsg{content: original})
		m = u.(reviewModel)
		m.cursor = 1
		if quick {
			m.cursor = 2
		}
		for _, key := range []tea.KeyType{tea.KeyEnter, tea.KeyEnter, tea.KeyEsc} {
			u, _ = m.Update(tea.KeyMsg{Type: key})
			m = u.(reviewModel)
		}
		if m.quitting || m.report != original || m.state == reviewStateAnalyzing {
			t.Errorf("quick=%v: regeneration cancel lost report: quitting=%v report=%q", quick, m.quitting, m.report)
		}
	}
}

func TestReview_CancelViewDetailsRestoresQuickReview(t *testing.T) {
	m := newTestReviewModel(true)
	originalSystem := m.initialMsgs[0].Content[0].Text
	u, _ := m.Update(reviewResultMsg{content: "Original quick review"})
	m = u.(reviewModel)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(reviewModel)
	stale := m.genID
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = u.(reviewModel)
	u, _ = m.Update(reviewResultMsg{content: "Late full review", genID: stale})
	m = u.(reviewModel)
	if m.quitting || !m.isQuickMode || m.report != "Original quick review" || m.state != reviewStateQuickDone {
		t.Fatalf("quick review lost: quitting=%v quick=%v report=%q state=%v", m.quitting, m.isQuickMode, m.report, m.state)
	}
	if got := m.initialMsgs[0].Content[0].Text; got != originalSystem {
		t.Fatalf("cancel kept full-review prompt: %q", got)
	}
	m.viewport.GotoBottom()
	if !strings.Contains(m.View(), "View Details") {
		t.Error("quick-review actions not restored")
	}
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
	m := newReviewModel(context.Background(), fakeProvider{resp: "```markdown\n## Conclusion\nok\n```"}, baseMsgs(), 0.7, 5*time.Second, true, tr)
	msg := m.generateReviewCmd()()
	res, ok := msg.(reviewResultMsg)
	if !ok {
		t.Fatalf("expected reviewResultMsg, got %T", msg)
	}
	if res.err != nil || !strings.Contains(res.content, "Conclusion") {
		t.Errorf("unexpected result: content=%q err=%v", res.content, res.err)
	}
}

func TestReview_EmptyResponseIsError(t *testing.T) {
	for _, response := range []string{"", " \n\t", "```markdown\n\n```"} {
		m := newTestReviewModel(true)
		m.provider = fakeProvider{resp: response}
		result := m.generateReviewCmd()().(reviewResultMsg)
		if result.err == nil {
			t.Errorf("empty review accepted: content=%q", result.content)
		}
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
