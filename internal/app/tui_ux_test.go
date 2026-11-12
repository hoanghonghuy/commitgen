package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/validator"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

func TestValidation_AutoFixPreservesSubject(t *testing.T) {
	for _, prefix := range []string{"", " "} {
		t.Run("leading-space="+prefix, func(t *testing.T) {
			m := newTestModel()
			m.validator = validator.New(validator.Config{Rules: map[string]validator.RuleConfig{"subject-length": {Enabled: true, Max: 50}}})
			u, _ := m.Update(commitResultMsg{content: "feat: initial"})
			m = u.(tuiModel)
			m.cursor = 2
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = u.(tuiModel)
			m.textarea.SetValue(prefix + strings.Repeat("修正", 40))
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = u.(tuiModel)
			m.cursor = 0
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = u.(tuiModel)
			if m.state != stateValidationFailed {
				t.Fatal("did not reach validation screen")
			}
			u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
			m = u.(tuiModel)
			if m.state != stateConfirm || strings.TrimSpace(m.commitMsg) == "" || len([]rune(m.commitMsg)) > 50 {
				t.Errorf("Auto-fix erased nonblank subject: state=%v message=%q", m.state, m.commitMsg)
			}
		})
	}
}

func TestConfigFieldValidator_DefaultsAndErrors(t *testing.T) {
	for _, accessible := range []bool{false, true} {
		t.Run(fmt.Sprintf("accessible=%v", accessible), func(t *testing.T) {
			t.Setenv("TERM", "xterm")
			t.Setenv("ACCESSIBLE", "")
			if accessible {
				t.Setenv("ACCESSIBLE", "1")
			}
			value := "7"
			validate := configFieldValidator(&value, func(s string) error { return validateIntField(nil, s) })
			if err := validate(""); (err == nil) != accessible {
				t.Errorf("empty input validation changed: accessible=%v err=%v", accessible, err)
			}
			if err := validate("not-a-number"); err == nil {
				t.Error("invalid explicit number was accepted")
			}
			value = "invalid-default"
			if err := validate(""); err == nil {
				t.Error("invalid default was accepted")
			}
		})
	}
}

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

func TestStreamingView_FitsTerminalAndShowsLatestText(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleVI, i18n.LocaleJA, i18n.LocaleZH} {
		for _, size := range []struct{ width, height int }{{80, 24}, {40, 24}, {80, 12}} {
			t.Run(fmt.Sprintf("%s/%dx%d", locale, size.width, size.height), func(t *testing.T) {
				m := newTestModel()
				m.i18n = i18n.New(locale)
				u, _ := m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
				m = u.(tuiModel)
				u, _ = m.Update(streamEvent{delta: "feat: change\n\n" + strings.Repeat("Detailed body line\n", 80) + "LATEST_LINE"})
				m = u.(tuiModel)
				for _, resize := range []bool{false, true} {
					if resize {
						u, _ = m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height - 1})
						m = u.(tuiModel)
					}
					view := m.View()
					if lipgloss.Height(view) > m.height {
						t.Errorf("stream exceeds terminal: view=%d terminal=%d", lipgloss.Height(view), m.height)
					}
					if !strings.Contains(view, "LATEST_LINE") || !strings.Contains(view, strings.TrimSpace(m.i18n.T("tui.hint.esc_cancel"))) {
						t.Error("latest text or cancel hint is hidden")
					}
				}
			})
		}
	}
}

func TestClipboardError_ViewportFitsTerminal(t *testing.T) {
	clipErr := errors.New("No clipboard utilities available. Please install xsel, xclip, wl-clipboard or Termux:API add-on for termux-clipboard-get/set.")
	for _, review := range []bool{false, true} {
		for _, width := range []int{40, 80} {
			var m tea.Model
			if review {
				m = newReviewModel(context.Background(), fakeProvider{}, baseMsgs(), 0.7, time.Second, true, i18n.New(i18n.LocaleEN))
				m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				m, _ = m.Update(reviewResultMsg{content: strings.Repeat("Review warning detail\n", 80)})
			} else {
				m = newTestModel()
				m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				m, _ = m.Update(commitResultMsg{content: "feat: change\n\n" + strings.Repeat("Detailed body line\n", 80)})
			}
			m, _ = m.Update(clipboardErrMsg{err: clipErr})
			for _, height := range []int{24, 23} {
				m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
				view := m.View()
				if lipgloss.Height(view) > height {
					t.Errorf("review=%v width=%d: clipboard status overflows: height=%d terminal=%d", review, width, lipgloss.Height(view), height)
				}
				if !strings.Contains(view, "clipboard") || !strings.Contains(view, "Enter") {
					t.Error("status or keyboard footer lost")
				}
			}
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
	if !UseAccessibleForm(func(k string) string {
		if k == "ACCESSIBLE" {
			return "1"
		}
		return ""
	}) {
		t.Fatal("ACCESSIBLE should enable")
	}
	if !UseAccessibleForm(func(k string) string {
		if k == "TERM" {
			return "dumb"
		}
		return ""
	}) {
		t.Fatal("TERM=dumb should enable")
	}
	if UseAccessibleForm(func(string) string { return "" }) {
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
	for _, regenerate := range []bool{false, true} {
		m := newTestModel()
		m.state = stateChoose
		m.candidates = []string{"feat: a", "feat: b", "feat: c", "feat: d", "feat: e"}
		m.cursor = 4
		m.commitMsg = "feat: old"
		var u tea.Model = m
		if regenerate {
			u, _ = u.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		}
		u, _ = u.Update(tea.KeyMsg{Type: tea.KeyEsc})
		tm := u.(tuiModel)
		if tm.commitMsg != "feat: old" || tm.state != stateConfirm {
			t.Errorf("regenerate=%v: Esc applied candidate or failed to return: message=%q state=%v", regenerate, tm.commitMsg, tm.state)
		}
		if tm.cursor < 0 || tm.cursor >= confirmActionCount || !strings.Contains(tm.View(), "> ") {
			t.Errorf("regenerate=%v: invalid confirm selection: cursor=%d", regenerate, tm.cursor)
		}
	}
}

func TestRegenHint_FitsTerminalAfterPasteAndResize(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleVI, i18n.LocaleJA, i18n.LocaleZH} {
		for _, review := range []bool{false, true} {
			var model tea.Model
			if review {
				m := newTestReviewModel(false)
				m.tr = i18n.New(locale)
				m, _ = m.enterRegenHint()
				m.hintInput.SetValue(strings.Repeat("Keep the public API compatible. ", 40))
				model = m
			} else {
				m := newTestModel()
				m.i18n = i18n.New(locale)
				m.state = stateRegenHint
				m.hintInput.Focus()
				m.hintInput.SetValue(strings.Repeat("Keep the public API compatible. ", 40))
				model = m
			}
			for _, width := range []int{80, 40, 80} {
				model, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: 24})
				if view := model.View(); lipgloss.Height(view) > 24 || lipgloss.Width(view) > width {
					t.Errorf("locale=%s review=%v: guidance overflow %dx%d in %dx24", locale, review, lipgloss.Width(view), lipgloss.Height(view), width)
				}
			}
			var value string
			switch m := model.(type) {
			case tuiModel:
				value = m.hintInput.Value()
			case reviewModel:
				value = m.hintInput.Value()
			}
			if value != strings.Repeat("Keep the public API compatible. ", 40) {
				t.Error("resizing discarded guidance text")
			}
		}
	}
}

func TestClipboardCopy_DoesNotRunInsideUpdate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("clipboard fixture uses a Linux executable")
	}
	if os.Getenv("COMMITGEN_TEST_CLIPBOARD_CHILD") == "1" {
		marker := os.Getenv("COMMITGEN_TEST_CLIPBOARD_FILE")
		for _, review := range []bool{false, true} {
			var model tea.Model
			if review {
				m := newTestReviewModel(false)
				m.state, m.report = reviewStateDone, "clipboard fixture text"
				model = m
			} else {
				m := newTestModel()
				m.state, m.commitMsg = stateConfirm, "clipboard fixture text"
				model = m
			}
			_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("review=%v: clipboard ran inside Update: %v", review, err)
			}
			if cmd == nil {
				t.Fatal("missing clipboard command")
			}
			msg := cmd()
			switch msg.(type) {
			case copyDoneMsg, reviewCopyDoneMsg:
			default:
				t.Fatalf("copy failed: %T %v", msg, msg)
			}
			if content, err := os.ReadFile(marker); err != nil || string(content) != "clipboard fixture text" {
				t.Errorf("wrong clipboard content: %q err=%v", content, err)
			}
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "copied.txt")
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte("#!/bin/sh\n/bin/cat > "+shellQuote(marker)+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestClipboardCopy_DoesNotRunInsideUpdate$")
	cmd.Env = append(os.Environ(), "COMMITGEN_TEST_CLIPBOARD_CHILD=1", "COMMITGEN_TEST_CLIPBOARD_FILE="+marker, "WAYLAND_DISPLAY=", "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "GORACE=atexit_sleep_ms=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clipboard subprocess: %v\n%s", err, out)
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
