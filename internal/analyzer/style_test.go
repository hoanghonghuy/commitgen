package analyzer

import (
	"strings"
	"testing"
)

func TestAnalyzeCommitStyle_ConventionalScopesTickets(t *testing.T) {
	style := AnalyzeCommitStyle([]string{
		"feat(auth): add login PROJ-123",
		"fix(auth): handle expired token PROJ-124",
		"feat(ui): refresh banner",
		"docs: update readme",
	})

	if style.TotalCommits != 4 || style.ConventionalCount != 4 {
		t.Fatalf("unexpected conventional counts: %+v", style)
	}
	for _, want := range []string{"feat", "fix", "docs"} {
		if !contains(style.Types, want) {
			t.Errorf("types missing %q: %v", want, style.Types)
		}
	}
	if !contains(style.Scopes, "auth") || !contains(style.Scopes, "ui") {
		t.Errorf("scopes missing auth/ui: %v", style.Scopes)
	}
	if !style.UsesTickets || !contains(style.TicketExamples, "PROJ-123") {
		t.Errorf("ticket examples missing: %+v", style)
	}

	guidance := style.Guidance()
	for _, want := range []string{"Conventional Commits", "Common scopes", "Ticket references", "do not invent IDs"} {
		if !strings.Contains(guidance, want) {
			t.Errorf("guidance missing %q:\n%s", want, guidance)
		}
	}
}

func TestAnalyzeCommitStyle_EmojiPlacement(t *testing.T) {
	style := AnalyzeCommitStyle([]string{
		"✨ feat: add sparkle",
		"🐛 fix: squash bug",
		"✨ feat(ui): polish card",
	})
	if !style.UsesEmoji {
		t.Fatalf("expected emoji style: %+v", style)
	}
	if style.EmojiPlacement != "prefix" {
		t.Errorf("EmojiPlacement=%q", style.EmojiPlacement)
	}
	if !contains(style.Emojis, "✨") || !contains(style.Emojis, "🐛") {
		t.Errorf("emoji list=%v", style.Emojis)
	}
	if !strings.Contains(style.Guidance(), "Emoji style observed") {
		t.Errorf("guidance missing emoji line: %s", style.Guidance())
	}
}

func TestAnalyzeCommitStyle_Empty(t *testing.T) {
	style := AnalyzeCommitStyle(nil)
	if style.Guidance() != "" {
		t.Errorf("empty style guidance=%q", style.Guidance())
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
