package vscodeprompt

import (
	"fmt"
	"strings"
	"testing"
)

func TestExtractOneTextCodeBlock_NestedFences(t *testing.T) {
	body := "# PR title\n\n## Summary\n```go\nfunc main() {}\n```\n\n## Test plan\n- [x] go test ./..."
	for _, fence := range []string{"```", "````", "~~~~"} {
		t.Run(fmt.Sprintf("fence=%s", fence), func(t *testing.T) {
			raw := fence + "markdown\n" + body + "\n" + fence
			got, ok := ExtractOneTextCodeBlock(raw)
			want := body
			if fence == "```" {
				// Equal inner/outer fences are not one Markdown wrapper;
				// retain the full raw response instead of extracting a fragment.
				want = raw
			}
			if got != want || ok != (fence != "```") {
				t.Errorf("nested content lost: got=%q ok=%v", got, ok)
			}
		})
	}
	for _, raw := range []string{"Review:\n" + body, "```text\none\n```\n```text\ntwo\n```"} {
		got, ok := ExtractOneTextCodeBlock(raw)
		if ok || got != raw {
			t.Errorf("partial block treated as complete wrapper: got=%q ok=%v", got, ok)
		}
	}
}

func TestBuildReviewMessages_DefaultTemplate(t *testing.T) {
	data := Data{
		RepositoryName: "test-repo",
		BranchName:     "main",
		Changes:        []Change{{Path: "main.go", Diff: "package main"}},
	}

	msgs := BuildReviewMessages(data, false)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != 0 {
		t.Errorf("expected role 0 (system), got %d", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content[0].Text, "AI code reviewer") {
		t.Error("default review system prompt not found")
	}
	if !strings.Contains(msgs[1].Content[0].Text, "Review the staged CODE CHANGES") {
		t.Error("review user reminder not found")
	}
}

func TestBuildVSCodeMessages_DefaultTemplate(t *testing.T) {
	data := Data{
		RepositoryName: "test-repo",
		BranchName:     "main",
		Changes:        []Change{{Path: "main.go", Diff: "package main"}},
	}

	msgs := BuildVSCodeMessages(data)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != 0 { // System
		t.Errorf("expected role 0 (system), got %d", msgs[0].Role)
	}
	// Check if default text is present
	if !strings.Contains(msgs[0].Content[0].Text, "You are an AI programming assistant") {
		t.Error("default system prompt not found")
	}
}

func TestBuildVSCodeMessages_CustomTemplate(t *testing.T) {
	customTmpl := "Hello {{.RepositoryName}} on branch {{.BranchName}}"
	data := Data{
		RepositoryName:       "my&repo",
		BranchName:           "feature/<parser>",
		Changes:              []Change{{Path: "main.go", Diff: "package main"}},
		SystemPromptTemplate: customTmpl,
	}

	msgs := BuildVSCodeMessages(data)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	sysContent := msgs[0].Content[0].Text
	expected := "Hello my&repo on branch feature/<parser>"
	if sysContent != expected {
		t.Errorf("expected %q, got %q", expected, sysContent)
	}
}

func TestBuildVSCodeMessages_IncludesCommitTemplate(t *testing.T) {
	data := Data{
		RepositoryName: "test-repo",
		BranchName:     "main",
		CommitTemplate: "Subject\n\nBody\n\nSigned-off-by: Name <email>",
		Changes:        []Change{{Path: "main.go", Diff: "package main"}},
	}

	msgs := BuildVSCodeMessages(data)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	systemText := msgs[0].Content[0].Text
	for _, want := range []string{"Repository commit template", "Signed-off-by", "Follow its structure"} {
		if !strings.Contains(systemText, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestBuildVSCodeMessages_IncludesCommitStyleGuidance(t *testing.T) {
	data := Data{
		RepositoryName:      "test-repo",
		BranchName:          "main",
		CommitStyleGuidance: "- Recent commits mostly use Conventional Commits; prefer types: feat, fix.",
		Changes:             []Change{{Path: "main.go", Diff: "package main"}},
	}

	msgs := BuildVSCodeMessages(data)
	systemText := msgs[0].Content[0].Text
	for _, want := range []string{"Repository commit style", "prefer types: feat, fix"} {
		if !strings.Contains(systemText, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}
