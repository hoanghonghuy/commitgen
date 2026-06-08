package vscodeprompt

import (
	"strings"
	"testing"
)

func TestRenderTemplate_InvalidParseFallsBack(t *testing.T) {
	// Malformed template (unclosed action) should fall back to the raw string.
	raw := "hello {{.RepositoryName"
	got := renderTemplate(raw, Data{RepositoryName: "r"})
	if got != raw {
		t.Errorf("expected raw fallback, got %q", got)
	}
}

func TestRenderTemplate_ExecuteErrorFallsBack(t *testing.T) {
	// Referencing a non-existent field triggers an execution error → fallback to raw.
	raw := "value: {{.DoesNotExist}}"
	got := renderTemplate(raw, Data{})
	if got != raw {
		t.Errorf("expected raw fallback on execute error, got %q", got)
	}
}

func TestRenderTemplate_NoActionShortCircuit(t *testing.T) {
	raw := "plain text without actions"
	if got := renderTemplate(raw, Data{}); got != raw {
		t.Errorf("got %q", got)
	}
}

func TestBuildReviewMessages_QuickTemplate(t *testing.T) {
	d := Data{RepositoryName: "r", Changes: []Change{{Path: "a.go", Diff: "d"}}}
	msgs := BuildReviewMessages(d, true)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if !strings.Contains(msgs[0].Content[0].Text, "Conclusion") {
		t.Error("quick review prompt should mention Conclusion section")
	}
}

func TestSummarizeGo_SingleLineFunc(t *testing.T) {
	src := "package main\n\nfunc f() { return }\n\nfunc g() {\n\tx := 1\n\t_ = x\n}\n"
	out := BuildAttachment("/repo", "x.go", src, true)
	if !strings.Contains(out, "{…}") {
		t.Error("expected collapsed function bodies")
	}
	if strings.Contains(out, "x := 1") {
		t.Error("multi-line func body should be summarized away")
	}
}

func TestSummarizeGo_MultilineSignature(t *testing.T) {
	src := "package main\n\nfunc add(\n\ta int,\n\tb int,\n) int {\n\treturn a + b\n}\n"
	out := BuildAttachment("/repo", "x.go", src, true)
	if !strings.Contains(out, "{…}") {
		t.Error("expected collapsed multi-line signature function")
	}
	if strings.Contains(out, "return a + b") {
		t.Error("func body should be summarized away")
	}
}

func TestSummarizeGo_TypeAndImportBlocks(t *testing.T) {
	src := "package main\n\nimport (\n\t\"fmt\"\n)\n\ntype (\n\tA int\n\tB string\n)\n\nconst Pi = 3.14\n"
	out := BuildAttachment("/repo", "x.go", src, true)
	for _, want := range []string{"import (", "\"fmt\"", "type (", "A int", "const Pi"} {
		if !strings.Contains(out, want) {
			t.Errorf("summarized Go missing %q", want)
		}
	}
}
