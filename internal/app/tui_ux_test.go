package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/validator"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func TestCandidatePicker_FitsTerminalAndKeepsSelectionVisible(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {80, 12}, {40, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := newTestModel()
			u, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = u.(tuiModel)
			var contents []string
			for i := 0; i < 20; i++ {
				contents = append(contents, fmt.Sprintf("feat: option %02d\n\nDetailed body", i+1))
			}
			u, _ = m.Update(candidatesMsg{contents: contents})
			m = u.(tuiModel)
			if height := lipgloss.Height(m.View()); height > m.height {
				t.Errorf("picker height=%d exceeds terminal=%d", height, m.height)
			}
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			m = u.(tuiModel)
			if m.viewport.YOffset == 0 {
				t.Error("PgDn did not scroll candidates")
			}
			before := m.viewport.YOffset
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
			m = u.(tuiModel)
			if m.viewport.YOffset >= before {
				t.Error("PgUp did not scroll candidates back")
			}
			for i := 0; i < len(contents)-1; i++ {
				u, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m = u.(tuiModel)
			}
			if !strings.Contains(m.View(), "feat: option 20") {
				t.Error("selected candidate is outside viewport")
			}
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = u.(tuiModel)
			if m.commitMsg != contents[19] || m.state != stateConfirm {
				t.Errorf("selected wrong candidate: %q", m.commitMsg)
			}
		})
	}
}

func TestValidationScreen_FitsTerminalAndScrolls(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {80, 12}, {40, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := newTestModel()
			m.validator = validator.New(validator.Config{Rules: map[string]validator.RuleConfig{"body-line-length": {Enabled: true, Max: 10}}})
			u, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			m = u.(tuiModel)
			u, _ = m.Update(commitResultMsg{content: "feat: change\n\n" + strings.Repeat("Long body line with warning\n", 40)})
			m = u.(tuiModel)
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = u.(tuiModel)
			if m.state != stateValidationFailed {
				t.Fatal("did not reach validation screen")
			}
			if height := lipgloss.Height(m.View()); height > m.height {
				t.Errorf("validation height=%d exceeds terminal=%d", height, m.height)
			}
			m.viewport.GotoTop()
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			m = u.(tuiModel)
			if m.viewport.YOffset == 0 {
				t.Error("PgDn did not scroll issues")
			}
			before := m.viewport.YOffset
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
			m = u.(tuiModel)
			if m.viewport.YOffset >= before {
				t.Error("PgUp did not scroll issues back")
			}
			m.viewport.GotoTop()
			u, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
			m = u.(tuiModel)
			if m.viewport.YOffset == 0 {
				t.Error("mouse wheel did not scroll issues")
			}
			for i := 0; i < validationActionCount-1; i++ {
				u, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m = u.(tuiModel)
			}
			if !strings.Contains(m.View(), m.i18n.T("tui.action.cancel")) {
				t.Error("selected action is hidden")
			}
			u, _ = m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height + 1})
			m = u.(tuiModel)
			if lipgloss.Height(m.View()) > m.height || !strings.Contains(m.View(), m.i18n.T("tui.action.cancel")) {
				t.Error("validation viewport broken after resize")
			}
		})
	}
}

func TestActionFooter_IncludesNav(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	got := actionFooter(tr)
	for _, want := range []string{"↑", "↓", "Enter"} {
		if !strings.Contains(got, want) {
			t.Fatalf("footer %q missing %q", got, want)
		}
	}
}

func TestConfirm_NavigationKeepsWrappedActionsVisible(t *testing.T) {
	for _, content := range []string{
		"feat: explain change\n\n" + strings.Repeat("long wrapped body ", 150),
		"feat: explain change\n\n" + strings.Repeat("body line\n", 80),
	} {
		m := newTestModel()
		u, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = u.(tuiModel)
		u, _ = m.Update(commitResultMsg{content: content})
		m = u.(tuiModel)
		m.viewport.GotoBottom()
		for _, key := range []tea.KeyType{tea.KeyDown, tea.KeyDown, tea.KeyUp} {
			u, _ = m.Update(tea.KeyMsg{Type: key})
			m = u.(tuiModel)
			if !strings.Contains(m.View(), "Regenerate") || !strings.Contains(m.View(), "Edit") {
				t.Errorf("actions hidden: cursor=%d offset=%d", m.cursor, m.viewport.YOffset)
			}
		}
	}
}

func TestCandidatePreview_TwoLines(t *testing.T) {
	msg := "feat: one\n\nbody line\nmore"
	got := candidatePreview(msg, 2)
	if !strings.Contains(got, "feat: one") || !strings.Contains(got, "…") {
		t.Fatalf("got %q", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d: %q", len(lines), got)
	}
}

func TestCandidatesProgress(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	got := candidatesProgress(tr, 2, 5)
	if !strings.Contains(got, "2") || !strings.Contains(got, "5") {
		t.Fatalf("got %q", got)
	}
}

func TestShouldUseAccessibleForm(t *testing.T) {
	if !shouldUseAccessibleForm(func(k string) string {
		if k == "ACCESSIBLE" {
			return "1"
		}
		return ""
	}) {
		t.Fatal("ACCESSIBLE should enable")
	}
	if !shouldUseAccessibleForm(func(k string) string {
		if k == "TERM" {
			return "dumb"
		}
		return ""
	}) {
		t.Fatal("TERM=dumb should enable")
	}
	if shouldUseAccessibleForm(func(string) string { return "" }) {
		t.Fatal("default should be false")
	}
}

func TestAppendGuidanceMessage(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	base := []vscodeprompt.VSCodeMessage{{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "base"}}}}
	if got := appendGuidanceMessage(base, tr, ""); len(got) != 1 {
		t.Fatalf("empty hint should keep len=1, got %d", len(got))
	}
	got := appendGuidanceMessage(base, tr, "shorter")
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	text := got[1].Content[0].Text
	if !strings.Contains(text, "shorter") {
		t.Fatalf("text=%q", text)
	}
	if !strings.Contains(text, tr.T("tui.regen.guidance_prefix")) {
		t.Fatalf("expected i18n prefix in %q", text)
	}
}

func TestConfirmView_ShortMessageShowsActionFooter(t *testing.T) {
	m := newTestModel()
	u, _ := m.Update(commitResultMsg{content: "feat: short"})
	tm := u.(tuiModel)
	view := tm.View()
	if !strings.Contains(view, "Enter") {
		t.Fatalf("confirm view missing Enter in footer: %q", view)
	}
}

func TestClipboardErr_ShowsStatusBanner(t *testing.T) {
	m := newTestModel()
	u, _ := m.Update(commitResultMsg{content: "feat: x"})
	tm := u.(tuiModel)
	u, _ = tm.Update(clipboardErrMsg{err: errors.New("no clip")})
	tm = u.(tuiModel)
	if tm.state != stateConfirm {
		t.Fatalf("state=%v", tm.state)
	}
	view := tm.View()
	if !strings.Contains(view, "Clipboard") && !strings.Contains(view, "clipboard") {
		t.Fatalf("expected clipboard status in view: %q", view)
	}
}

func TestValidation_IgnoreBlockedShowsStatus(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	v := validator.New(validator.Config{Rules: map[string]validator.RuleConfig{
		"subject-length": {Enabled: true, Max: 5},
	}})
	m := newTuiModel(context.Background(), "/repo", fakeProvider{}, baseMsgs(), 0.7, time.Second, false, "", tr, v)
	m.commitMsg = "feat: this subject is way too long for max five"
	m.validationIssues = v.Validate(m.commitMsg)
	m.state = stateValidationFailed
	m.cursor = 2 // Ignore
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm := u.(tuiModel)
	if tm.state != stateValidationFailed {
		t.Fatalf("should stay on validation, got %v", tm.state)
	}
	view := tm.View()
	if !strings.Contains(strings.ToLower(view), "error") && !strings.Contains(view, "Ignore") {
		// status should mention fixing errors
		if tm.statusBanner == "" {
			t.Fatal("expected statusBanner when ignore blocked")
		}
	}
	if tm.statusBanner == "" {
		t.Fatal("expected statusBanner when ignore blocked")
	}
}

func TestValidation_AutofixNoopShowsStatus(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	// required-patterns cannot autofix → AutoFix returns unchanged
	v := validator.New(validator.Config{Rules: map[string]validator.RuleConfig{
		"required-patterns": {Enabled: true, Patterns: []string{`TICKET-\d+`}},
	}})
	m := newTuiModel(context.Background(), "/repo", fakeProvider{}, baseMsgs(), 0.7, time.Second, false, "", tr, v)
	m.commitMsg = "feat: no ticket"
	m.validationIssues = v.Validate(m.commitMsg)
	m.state = stateValidationFailed
	m.cursor = 0 // Auto-fix
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm := u.(tuiModel)
	if tm.statusBanner == "" {
		t.Fatal("expected statusBanner when autofix makes no change")
	}
}

func TestCommitDone_HoldsBeforeQuit(t *testing.T) {
	prev := outcomeHoldDuration
	outcomeHoldDuration = 0
	defer func() { outcomeHoldDuration = prev }()

	m := newTestModel()
	u, cmd := m.Update(commitDoneMsg{})
	tm := u.(tuiModel)
	if tm.state != stateDone {
		t.Fatalf("state=%v", tm.state)
	}
	if cmd == nil {
		t.Fatal("expected hold cmd")
	}
	// Immediate hold (duration 0) yields outcomeHoldDoneMsg
	msg := cmd()
	if _, ok := msg.(outcomeHoldDoneMsg); !ok {
		t.Fatalf("got %T", msg)
	}
	u, cmd = tm.Update(msg)
	tm = u.(tuiModel)
	if !tm.quitting || cmd == nil {
		t.Fatal("hold done should quit")
	}
}

func TestEscCancelsGenerating(t *testing.T) {
	m := newTestModel()
	m.state = stateGenerating
	m.commitMsg = "feat: previous"
	cancelled := false
	m.genCancel = func() { cancelled = true }
	m.genID = 3
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm := u.(tuiModel)
	if !cancelled {
		t.Fatal("expected genCancel called")
	}
	if tm.state != stateConfirm {
		t.Fatalf("expected confirm after cancel, got %v", tm.state)
	}
	// Late result with old id ignored
	u, _ = tm.Update(commitResultMsg{content: "late", genID: 3})
	tm = u.(tuiModel)
	if tm.commitMsg == "late" {
		t.Fatal("late result must be ignored")
	}
}

func TestChooseEsc_DoesNotSelect(t *testing.T) {
	m := newTestModel()
	m.state = stateChoose
	m.candidates = []string{"feat: a", "feat: b"}
	m.cursor = 1
	m.commitMsg = "feat: old"
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm := u.(tuiModel)
	if tm.commitMsg != "feat: old" {
		t.Fatalf("Esc must not apply candidate, got %q", tm.commitMsg)
	}
	if tm.state != stateConfirm {
		t.Fatalf("state=%v", tm.state)
	}
}

func TestChoosePreview_InView(t *testing.T) {
	m := newTestModel()
	m.state = stateChoose
	m.candidates = []string{"feat: one\n\nbody line\nmore"}
	m.cursor = 0
	view := m.View()
	if !strings.Contains(view, "feat: one") || !strings.Contains(view, "…") {
		t.Fatalf("choose view should show multi-line preview: %q", view)
	}
}

func TestCandidatesProgress_InGeneratingView(t *testing.T) {
	m := newTestModel()
	m.state = stateGenerating
	m.count = 3
	m.candCurrent = 2
	m.candTotal = 3
	view := m.View()
	if !strings.Contains(view, "2") || !strings.Contains(view, "3") {
		t.Fatalf("expected i/N progress in view: %q", view)
	}
}

func TestReview_RegenHintState(t *testing.T) {
	m := newTestReviewModel(false)
	u, _ := m.Update(reviewResultMsg{content: "## ok"})
	rm := u.(reviewModel)
	rm.cursor = 1 // Regenerate
	u, _ = rm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm = u.(reviewModel)
	if rm.state != reviewStateRegenHint {
		t.Fatalf("expected regen hint, got %v", rm.state)
	}
}

func TestReview_EscCancelsAnalyzing(t *testing.T) {
	m := newTestReviewModel(true)
	m.state = reviewStateAnalyzing
	m.report = "previous"
	cancelled := false
	m.genCancel = func() { cancelled = true }
	m.genID = 2
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	rm := u.(reviewModel)
	if !cancelled {
		t.Fatal("expected cancel")
	}
	if rm.state != reviewStateQuickDone {
		t.Fatalf("state=%v", rm.state)
	}
	u, _ = rm.Update(reviewResultMsg{content: "late", genID: 2})
	rm = u.(reviewModel)
	if rm.report == "late" {
		t.Fatal("late result must be ignored")
	}
}

func TestApplyFormAccessibility(t *testing.T) {
	form := applyFormAccessibility(nil, func(string) string { return "1" })
	if form != nil {
		t.Fatal("nil form stays nil")
	}
}

func TestParseIntFieldError_I18n(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	err := validateIntField(tr, "abc")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "strconv") {
		t.Fatalf("should not expose strconv: %v", err)
	}
}

func TestPrintDurableOutcome_SuccessAndError(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	var buf strings.Builder
	printDurableOutcome(&buf, tr, nil)
	if !strings.Contains(buf.String(), "Committed") && !strings.Contains(buf.String(), tr.T("tui.state.success")) {
		t.Fatalf("success line: %q", buf.String())
	}
	buf.Reset()
	printDurableOutcome(&buf, tr, errors.New("boom"))
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("error line: %q", buf.String())
	}
}

func TestCommitDone_SetsReachedDone(t *testing.T) {
	prev := outcomeHoldDuration
	outcomeHoldDuration = 0
	defer func() { outcomeHoldDuration = prev }()
	m := newTestModel()
	u, _ := m.Update(commitDoneMsg{})
	tm := u.(tuiModel)
	if !tm.reachedDone {
		t.Fatal("expected reachedDone after commitDoneMsg")
	}
}

type captureProvider struct {
	lastMsgs []vscodeprompt.VSCodeMessage
	resp     string
}

func (c *captureProvider) Generate(_ context.Context, msgs []vscodeprompt.VSCodeMessage, _ float64) (string, error) {
	c.lastMsgs = append([]vscodeprompt.VSCodeMessage{}, msgs...)
	if c.resp == "" {
		c.resp = "## ok"
	}
	return c.resp, nil
}

func TestReview_GenerateIncludesGuidance(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	cap := &captureProvider{resp: "```markdown\n## Conclusion\nok\n```"}
	m := newReviewModel(context.Background(), cap, baseMsgs(), 0.7, time.Second, false, tr)
	m.regenHint = "focus on security"
	msg := m.generateReviewCmd()()
	res, ok := msg.(reviewResultMsg)
	if !ok || res.err != nil {
		t.Fatalf("unexpected result: %#v", msg)
	}
	if len(cap.lastMsgs) < 2 {
		t.Fatalf("expected guidance appended, got %d msgs", len(cap.lastMsgs))
	}
	last := cap.lastMsgs[len(cap.lastMsgs)-1].Content[0].Text
	if !strings.Contains(last, "focus on security") {
		t.Fatalf("guidance missing in provider msgs: %q", last)
	}
}
