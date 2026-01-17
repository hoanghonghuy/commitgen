package vscodeprompt

import (
	"strings"
	"testing"
)

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
		RepositoryName:       "my-repo",
		BranchName:           "dev",
		Changes:              []Change{{Path: "main.go", Diff: "package main"}},
		SystemPromptTemplate: customTmpl,
	}

	msgs := BuildVSCodeMessages(data)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	sysContent := msgs[0].Content[0].Text
	expected := "Hello my-repo on branch dev"
	if sysContent != expected {
		t.Errorf("expected %q, got %q", expected, sysContent)
	}
}
