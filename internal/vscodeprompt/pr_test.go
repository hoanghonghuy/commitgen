package vscodeprompt

import (
	"strings"
	"testing"
)

func TestBuildPRMessages_DefaultTemplate(t *testing.T) {
	data := Data{
		RepositoryName: "test-repo",
		BranchName:     "feature/auth",
		BaseBranch:     "main",
		RecentRepoCommits: []string{
			"feat(auth): add login",
			"fix(auth): handle nil token",
		},
		Changes: []Change{{Path: "auth.go", Diff: "+func Login() {}"}},
	}

	msgs := BuildPRMessages(data)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	sys := msgs[0].Content[0].Text
	user := msgs[1].Content[0].Text
	if !strings.Contains(sys, "pull request") && !strings.Contains(sys, "Pull Request") {
		t.Errorf("system prompt should mention PR, got: %s", sys[:min(120, len(sys))])
	}
	if !strings.Contains(user, "feature/auth") || !strings.Contains(user, "main") {
		t.Error("user text should include branch and base")
	}
	if !strings.Contains(user, "auth.go") {
		t.Error("user text should include changed files")
	}
	if !strings.Contains(user, "feat(auth): add login") {
		t.Error("user text should include commits since base")
	}
}

func TestParsePROutput(t *testing.T) {
	raw := "```markdown\n# Add OAuth login\n\n## Summary\n- Add OAuth provider\n\n## Test plan\n- [ ] Login flow\n```\n"
	title, body, ok := ParsePROutput(raw)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if title != "Add OAuth login" {
		t.Errorf("title = %q", title)
	}
	if !strings.Contains(body, "## Summary") || !strings.Contains(body, "Test plan") {
		t.Errorf("body = %q", body)
	}

	plain := "Short title only"
	title, body, ok = ParsePROutput(plain)
	if !ok || title != "Short title only" || body != "" {
		t.Errorf("plain parse: title=%q body=%q ok=%v", title, body, ok)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
