package validator

import (
	"fmt"
	"regexp"
	"strings"
)

// Issue represents a validation problem found in a commit message.
type Issue struct {
	Level   string // "error", "warning"
	Message string
	Line    int
	Column  int
	Rule    string
}

// Rule defines the interface for a commit message validation rule.
type Rule interface {
	Validate(msg string) []Issue
	AutoFix(msg string) (string, bool) // returns (fixed, canAutoFix)
}

// RuleConfig holds configuration for a single validation rule.
type RuleConfig struct {
	Enabled  bool     `json:"enabled"`
	Max      int      `json:"max,omitempty"`
	Footers  []string `json:"footers,omitempty"`
	Words    []string `json:"words,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
}

// Config holds the complete validation configuration.
type Config struct {
	Rules map[string]RuleConfig `json:"rules"`
}

// Validator runs a set of rules against a commit message.
type Validator struct {
	rules []Rule
}

// New creates a Validator from the given configuration. Only enabled rules
// are added.
func New(cfg Config) *Validator {
	var rules []Rule

	if r, ok := cfg.Rules["subject-length"]; ok && r.Enabled {
		maxLen := r.Max
		if maxLen <= 0 {
			maxLen = 50
		}
		rules = append(rules, &SubjectLengthRule{MaxLength: maxLen})
	}
	if r, ok := cfg.Rules["body-line-length"]; ok && r.Enabled {
		maxLen := r.Max
		if maxLen <= 0 {
			maxLen = 72
		}
		rules = append(rules, &BodyLineLengthRule{MaxLength: maxLen})
	}
	if r, ok := cfg.Rules["required-footers"]; ok && r.Enabled {
		rules = append(rules, &RequiredFootersRule{Footers: r.Footers})
	}
	if r, ok := cfg.Rules["prohibited-words"]; ok && r.Enabled {
		rules = append(rules, &ProhibitedWordsRule{Words: r.Words})
	}
	if r, ok := cfg.Rules["required-patterns"]; ok && r.Enabled {
		rules = append(rules, &RequiredPatternsRule{Patterns: r.Patterns})
	}

	return &Validator{rules: rules}
}

// Enabled reports whether any validation rules are active.
func (v *Validator) Enabled() bool {
	return v != nil && len(v.rules) > 0
}

// Validate runs all enabled rules and returns all issues found.
func (v *Validator) Validate(msg string) []Issue {
	if v == nil {
		return nil
	}
	var all []Issue
	for _, r := range v.rules {
		all = append(all, r.Validate(msg)...)
	}
	return all
}

// HasErrors reports whether any issue is at error level.
func HasErrors(issues []Issue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}

// runeLen returns the number of Unicode runes in s.
func runeLen(s string) int {
	return len([]rune(s))
}

// AutoFix attempts to automatically fix all issues. It returns the fixed
// message and whether any changes were made.
func (v *Validator) AutoFix(msg string) (string, bool) {
	changed := false
	current := msg
	for _, r := range v.rules {
		if fixed, ok := r.AutoFix(current); ok {
			current = fixed
			changed = true
		}
	}
	return current, changed
}

// SubjectLengthRule validates that the first line (subject) does not exceed
// a maximum length.
type SubjectLengthRule struct {
	MaxLength int
}

func (r *SubjectLengthRule) Validate(msg string) []Issue {
	lines := strings.Split(msg, "\n")
	subject := lines[0]
	if runeLen(subject) > r.MaxLength {
		return []Issue{{
			Level:   "error",
			Message: fmt.Sprintf("Subject line too long: %d > %d", runeLen(subject), r.MaxLength),
			Line:    1,
			Column:  r.MaxLength + 1,
			Rule:    "subject-length",
		}}
	}
	return nil
}

func (r *SubjectLengthRule) AutoFix(msg string) (string, bool) {
	lines := strings.Split(msg, "\n")
	subject := lines[0]
	if runeLen(subject) <= r.MaxLength {
		return msg, false
	}
	runes := []rune(subject)
	cut := r.MaxLength
	for i := r.MaxLength - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			cut = i
			break
		}
	}
	lines[0] = string(runes[:cut])
	return strings.Join(lines, "\n"), true
}

// BodyLineLengthRule validates that body lines do not exceed a maximum length.
// The subject line and blank lines are ignored.
type BodyLineLengthRule struct {
	MaxLength int
}

func (r *BodyLineLengthRule) Validate(msg string) []Issue {
	lines := strings.Split(msg, "\n")
	var issues []Issue
	inBody := false
	for i, line := range lines {
		if i == 0 {
			continue // skip subject
		}
		if !inBody && strings.TrimSpace(line) == "" {
			continue // skip blank lines between subject and body
		}
		inBody = true
		if runeLen(line) > r.MaxLength {
			issues = append(issues, Issue{
				Level:   "warning",
				Message: fmt.Sprintf("Body line %d too long: %d > %d", i+1, runeLen(line), r.MaxLength),
				Line:    i + 1,
				Column:  r.MaxLength + 1,
				Rule:    "body-line-length",
			})
		}
	}
	return issues
}

func (r *BodyLineLengthRule) AutoFix(msg string) (string, bool) {
	// Auto-fix for body line length is complex (word wrapping); skip for now.
	return msg, false
}

// RequiredFootersRule validates that specified footers are present in the
// commit message.
type RequiredFootersRule struct {
	Footers []string
}

func (r *RequiredFootersRule) Validate(msg string) []Issue {
	var issues []Issue
	for _, footer := range r.Footers {
		if !strings.Contains(msg, footer+":") {
			issues = append(issues, Issue{
				Level:   "warning",
				Message: fmt.Sprintf("Missing footer: %s", footer),
				Line:    0,
				Column:  0,
				Rule:    "required-footers",
			})
		}
	}
	return issues
}

func (r *RequiredFootersRule) AutoFix(msg string) (string, bool) {
	// Cannot auto-fix missing footers without user identity.
	return msg, false
}

// ProhibitedWordsRule validates that the message does not contain prohibited
// words (case-insensitive).
type ProhibitedWordsRule struct {
	Words []string
}

func (r *ProhibitedWordsRule) Validate(msg string) []Issue {
	lower := strings.ToLower(msg)
	var issues []Issue
	for _, word := range r.Words {
		if strings.Contains(lower, strings.ToLower(word)) {
			issues = append(issues, Issue{
				Level:   "error",
				Message: fmt.Sprintf("Prohibited word found: %q", word),
				Line:    0,
				Column:  0,
				Rule:    "prohibited-words",
			})
		}
	}
	return issues
}

func (r *ProhibitedWordsRule) AutoFix(msg string) (string, bool) {
	changed := false
	current := msg
	for _, word := range r.Words {
		// Case-insensitive replace
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(word))
		if re.MatchString(current) {
			current = re.ReplaceAllString(current, "")
			changed = true
		}
	}
	// Clean up double spaces and leading/trailing whitespace
	if changed {
		current = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(current), " ")
	}
	return current, changed
}

// RequiredPatternsRule validates that the subject line matches at least one
// of the required regex patterns.
type RequiredPatternsRule struct {
	Patterns []string
}

func (r *RequiredPatternsRule) Validate(msg string) []Issue {
	lines := strings.Split(msg, "\n")
	subject := lines[0]
	for _, pat := range r.Patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		if re.MatchString(subject) {
			return nil
		}
	}
	return []Issue{{
		Level:   "error",
		Message: "Subject does not match any required pattern (e.g. Conventional Commits)",
		Line:    1,
		Column:  0,
		Rule:    "required-patterns",
	}}
}

func (r *RequiredPatternsRule) AutoFix(msg string) (string, bool) {
	// Cannot auto-fix pattern mismatches.
	return msg, false
}

// DefaultConfig returns the default validation configuration with sensible
// defaults for Conventional Commits.
func DefaultConfig() *Config {
	return &Config{
		Rules: map[string]RuleConfig{
			"subject-length":    {Enabled: true, Max: 50},
			"body-line-length":  {Enabled: true, Max: 72},
			"required-footers":  {Enabled: false},
			"prohibited-words":  {Enabled: false},
			"required-patterns": {Enabled: false},
		},
	}
}

// LoadConfig merges a custom config on top of defaults. When custom is nil,
// only defaults are returned.
func LoadConfig(custom *Config) *Config {
	defaults := DefaultConfig()
	if custom == nil {
		return defaults
	}
	if custom.Rules == nil {
		custom.Rules = make(map[string]RuleConfig)
	}
	for k, v := range defaults.Rules {
		if _, ok := custom.Rules[k]; !ok {
			custom.Rules[k] = v
		}
	}
	return custom
}
