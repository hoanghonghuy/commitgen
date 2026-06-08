package vscodeprompt

import (
	"strings"
	"testing"
)

func TestExtractOneTextCodeBlock(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantOK  bool
	}{
		{"text block", "```text\nfeat: add x\n```", "feat: add x", true},
		{"language block", "```go\npackage main\n```", "package main", true},
		{"no language", "```\nhello\n```", "hello", true},
		{"surrounding whitespace", "  \n```text\nfix: y\n```\n ", "fix: y", true},
		{"no block", "just prose here", "just prose here", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractOneTextCodeBlock(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ExtractOneTextCodeBlock(%q) = (%q,%v); want (%q,%v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestToOpenAIMessages_RoleMapping(t *testing.T) {
	in := []VSCodeMessage{
		{Role: RoleSystem, Content: []VSCodeContentPart{{Type: 1, Text: "s"}}},
		{Role: RoleUser, Content: []VSCodeContentPart{{Type: 1, Text: "u"}}},
		{Role: RoleAssistant, Content: []VSCodeContentPart{{Type: 1, Text: "a"}}},
		{Role: RoleUser, Content: []VSCodeContentPart{{Type: 0, Text: "ignored"}, {Type: 1, Text: "kept"}}},
	}
	out := ToOpenAIMessages(in)
	if len(out) != 4 {
		t.Fatalf("got %d messages", len(out))
	}
	if out[0].Role != "system" || out[1].Role != "user" || out[2].Role != "assistant" {
		t.Errorf("role mapping wrong: %+v", out)
	}
	// Type != 1 parts are skipped
	if out[3].Content != "kept" {
		t.Errorf("expected only text-type parts, got %q", out[3].Content)
	}
}

func TestBuildUserText_IncludesCommitsAndInstructions(t *testing.T) {
	d := Data{
		RepositoryName:     "repo",
		BranchName:         "dev",
		RecentUserCommits:  []string{"feat: a"},
		RecentRepoCommits:  []string{"fix: b"},
		Changes:            []Change{{Path: "x.go", Diff: "diff", OriginalCode: "orig"}},
		CustomInstructions: "be terse",
	}
	out := buildUserText(d)
	for _, want := range []string{"repo", "dev", "feat: a", "fix: b", "be terse", "```diff", "orig"} {
		if !strings.Contains(out, want) {
			t.Errorf("buildUserText missing %q", want)
		}
	}
}

func TestBuildReviewUserText_VietnameseLanguage(t *testing.T) {
	d := Data{
		RepositoryName: "repo",
		Changes:        []Change{{Path: "x.go", Diff: "d"}},
		ReviewLanguage: "vi",
	}
	out := buildReviewUserText(d)
	if !strings.Contains(out, "Vietnamese") {
		t.Error("expected Vietnamese language directive")
	}

	d.ReviewLanguage = "en"
	out = buildReviewUserText(d)
	if !strings.Contains(out, "English") {
		t.Error("expected English language directive")
	}
}

func TestBuildAttachment_NoSummarize(t *testing.T) {
	content := "line1\nline2\nline3"
	out := BuildAttachment("/repo", "main.go", content, false)
	if !strings.Contains(out, "isSummarized=\"false\"") {
		t.Error("expected isSummarized=false")
	}
	for _, want := range []string{"line1", "line2", "line3", "// filepath:"} {
		if !strings.Contains(out, want) {
			t.Errorf("attachment missing %q", want)
		}
	}
}

func TestBuildAttachment_SummarizeGo(t *testing.T) {
	goSrc := `package main

import (
	"fmt"
)

// Greeter does things
type Greeter struct {
	Name string
}

func main() {
	fmt.Println("hello")
	x := 1
	_ = x
}
`
	out := BuildAttachment("/repo", "main.go", goSrc, true)
	if !strings.Contains(out, "isSummarized=\"true\"") {
		t.Error("expected isSummarized=true")
	}
	// package line kept, func body collapsed
	if !strings.Contains(out, "package main") {
		t.Error("package line should be kept")
	}
	if !strings.Contains(out, "{…}") {
		t.Error("func body should be collapsed to {…}")
	}
	// inner body line should not appear
	if strings.Contains(out, `fmt.Println("hello")`) {
		t.Error("func body content should be summarized away")
	}
}

func TestBuildAttachment_SummarizeMarkdown(t *testing.T) {
	lines := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		lines = append(lines, "para "+string(rune('a'+i%26)))
	}
	md := strings.Join(lines, "\n")
	out := BuildAttachment("/repo", "doc.md", md, true)
	if !strings.Contains(out, "<!-- filepath:") {
		t.Error("markdown filepath comment expected")
	}
	if !strings.Contains(out, "isSummarized=\"true\"") {
		t.Error("expected summarized markdown")
	}
}

func TestFilepathCommentLine(t *testing.T) {
	cases := map[string]string{
		"a.md":  "<!--",
		"a.go":  "//",
		"a.py":  "#",
		"a.sh":  "#",
		"a.yml": "<!--",
	}
	for rel, prefix := range cases {
		got := filepathCommentLine(rel, "/abs/"+rel)
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("filepathCommentLine(%q) = %q; want prefix %q", rel, got, prefix)
		}
	}
}
