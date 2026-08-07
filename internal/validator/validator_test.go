package validator

import (
	"strings"
	"testing"
)

func TestHasErrors(t *testing.T) {
	issues := []Issue{{Level: "warning"}}
	if HasErrors(issues) {
		t.Error("warnings alone should not count as errors")
	}
	issues = append(issues, Issue{Level: "error"})
	if !HasErrors(issues) {
		t.Error("expected errors when an error-level issue is present")
	}
}

func TestValidator_EnabledAndNilSafe(t *testing.T) {
	var nilValidator *Validator
	if nilValidator.Enabled() {
		t.Error("nil validator should be disabled")
	}
	if issues := nilValidator.Validate("feat: x"); issues != nil {
		t.Errorf("nil validator Validate=%v", issues)
	}
	if fixed, changed := nilValidator.AutoFix("feat: x"); fixed != "feat: x" || changed {
		t.Errorf("nil validator AutoFix=(%q,%v)", fixed, changed)
	}

	empty := New(Config{})
	if empty.Enabled() {
		t.Error("validator without enabled rules should be disabled")
	}

	active := New(Config{Rules: map[string]RuleConfig{"subject-length": {Enabled: true, Max: 10}}})
	if !active.Enabled() {
		t.Error("validator with enabled rules should be enabled")
	}
}

func TestSubjectLengthRule_UnicodeRunes(t *testing.T) {
	r := SubjectLengthRule{MaxLength: 5}
	// 5 Vietnamese characters — should pass (count runes, not bytes).
	subject := "ăâêôư"
	if issues := r.Validate(subject); len(issues) != 0 {
		t.Fatalf("expected pass for 5 runes, got %+v", issues)
	}
	long := subject + "x"
	if issues := r.Validate(long); len(issues) == 0 {
		t.Fatal("expected fail when rune count exceeds max")
	}
}

func TestSubjectLengthRule_Validate_Pass(t *testing.T) {
	r := SubjectLengthRule{MaxLength: 50}
	issues := r.Validate("feat: add login")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d: %+v", len(issues), issues)
	}
}

func TestSubjectLengthRule_Validate_Fail(t *testing.T) {
	r := SubjectLengthRule{MaxLength: 50}
	longSubject := "feat: " + strings.Repeat("x", 50)
	issues := r.Validate(longSubject)
	if len(issues) == 0 {
		t.Error("expected issues for long subject, got none")
	}
	if len(issues) > 0 && issues[0].Level != "error" {
		t.Errorf("expected error level, got %q", issues[0].Level)
	}
}

func TestSubjectLengthRule_AutoFix(t *testing.T) {
	r := SubjectLengthRule{MaxLength: 50}
	longSubject := "feat: " + strings.Repeat("x", 50)
	fixed, ok := r.AutoFix(longSubject)
	if !ok {
		t.Error("expected AutoFix to succeed")
	}
	if len(fixed) > 50 {
		t.Errorf("expected fixed length <= 50, got %d: %q", len(fixed), fixed)
	}
}

func TestBodyLineLengthRule_Validate(t *testing.T) {
	r := BodyLineLengthRule{MaxLength: 72}
	// Short lines pass
	issues := r.Validate("feat: short\n\nbody line that is short")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
	// Long body line fails
	longBody := "feat: test\n\n" + strings.Repeat("x", 80)
	issues = r.Validate(longBody)
	if len(issues) == 0 {
		t.Error("expected issues for long body line")
	}
	if fixed, ok := r.AutoFix(longBody); fixed != longBody || ok {
		t.Errorf("body line AutoFix should be no-op, got (%q,%v)", fixed, ok)
	}
}

func TestRequiredFootersRule_Validate(t *testing.T) {
	r := RequiredFootersRule{Footers: []string{"Signed-off-by"}}
	// Missing footer
	issues := r.Validate("feat: test")
	if len(issues) == 0 {
		t.Error("expected issues for missing footer")
	}
	// Has footer
	issues = r.Validate("feat: test\n\nSigned-off-by: John Doe")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
	if fixed, ok := r.AutoFix("feat: test"); fixed != "feat: test" || ok {
		t.Errorf("required footer AutoFix should be no-op, got (%q,%v)", fixed, ok)
	}
}

func TestProhibitedWordsRule_Validate(t *testing.T) {
	r := ProhibitedWordsRule{Words: []string{"WIP", "temp", "debug"}}
	issues := r.Validate("feat: add login")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
	issues = r.Validate("WIP: work in progress")
	if len(issues) == 0 {
		t.Error("expected issues for prohibited word")
	}
}

func TestProhibitedWordsRule_AutoFix(t *testing.T) {
	r := ProhibitedWordsRule{Words: []string{"WIP"}}
	fixed, ok := r.AutoFix("WIP: work in progress")
	if !ok {
		t.Error("expected AutoFix to succeed")
	}
	if strings.Contains(fixed, "WIP") {
		t.Errorf("expected WIP to be removed, got %q", fixed)
	}
}

func TestRequiredPatternsRule_Validate(t *testing.T) {
	r := RequiredPatternsRule{Patterns: []string{`^(feat|fix|docs|style|refactor|test|chore)(\(.+\))?: .+`}}
	// Valid conventional commit
	issues := r.Validate("feat(auth): add login")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
	// Invalid format
	issues = r.Validate("added login feature")
	if len(issues) == 0 {
		t.Error("expected issues for non-conventional commit")
	}
	if fixed, ok := r.AutoFix("added login feature"); fixed != "added login feature" || ok {
		t.Errorf("required pattern AutoFix should be no-op, got (%q,%v)", fixed, ok)
	}

	invalidPattern := RequiredPatternsRule{Patterns: []string{"["}}
	if issues := invalidPattern.Validate("feat: still rejected"); len(issues) == 0 {
		t.Error("invalid required pattern should be ignored and reject message")
	}
}

func TestValidator_Validate_AllRules(t *testing.T) {
	v := New(Config{
		Rules: map[string]RuleConfig{
			"subject-length":   {Enabled: true, Max: 50},
			"body-line-length": {Enabled: true, Max: 72},
			"required-footers": {Enabled: true, Footers: []string{"Signed-off-by"}},
		},
	})
	// Valid message
	issues := v.Validate("feat: add login\n\nSigned-off-by: John Doe")
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d: %+v", len(issues), issues)
	}
	// Invalid message
	issues = v.Validate(strings.Repeat("x", 60))
	if len(issues) == 0 {
		t.Error("expected issues for invalid message")
	}
}

func TestValidator_Validate_DisabledRule(t *testing.T) {
	v := New(Config{
		Rules: map[string]RuleConfig{
			"subject-length": {Enabled: false, Max: 50},
		},
	})
	issues := v.Validate(strings.Repeat("x", 60))
	if len(issues) != 0 {
		t.Errorf("expected no issues for disabled rule, got %d", len(issues))
	}
}

func TestValidator_AutoFix(t *testing.T) {
	v := New(Config{
		Rules: map[string]RuleConfig{
			"subject-length":   {Enabled: true, Max: 50},
			"prohibited-words": {Enabled: true, Words: []string{"WIP"}},
		},
	})
	msg := "WIP: " + strings.Repeat("x", 60)
	fixed, changed := v.AutoFix(msg)
	if !changed {
		t.Error("expected AutoFix to make changes")
	}
	if len(fixed) > 50 {
		t.Errorf("expected fixed length <= 50, got %d", len(fixed))
	}
	if strings.Contains(fixed, "WIP") {
		t.Errorf("expected WIP to be removed, got %q", fixed)
	}
}

func TestLoadConfig_Default(t *testing.T) {
	cfg := LoadConfig(nil)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if !cfg.Rules["subject-length"].Enabled {
		t.Error("expected subject-length rule enabled by default")
	}
}

func TestLoadConfig_Merge(t *testing.T) {
	defaults := DefaultConfig()
	custom := &Config{
		Rules: map[string]RuleConfig{
			"subject-length": {Enabled: true, Max: 72},
		},
	}
	cfg := LoadConfig(custom)
	if cfg.Rules["subject-length"].Max != 72 {
		t.Errorf("expected custom max 72, got %d", cfg.Rules["subject-length"].Max)
	}
	_ = defaults
}
