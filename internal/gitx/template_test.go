package gitx

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetCommitTemplate_ConfiguredRelativePath(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "templates/commit.txt", "Subject\n\nBody\n")
	runGit(t, dir, "config", "commit.template", "templates/commit.txt")

	got, err := GetCommitTemplate(ctx, dir)
	if err != nil {
		t.Fatalf("GetCommitTemplate error: %v", err)
	}
	if got != "Subject\n\nBody" {
		t.Errorf("template = %q", got)
	}
}

func TestGetCommitTemplate_ConfiguredAbsolutePath(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	abs := filepath.Join(dir, "commit-template.txt")
	writeFile(t, dir, "commit-template.txt", "Header\nFooter\n")
	runGit(t, dir, "config", "commit.template", abs)

	got, err := GetCommitTemplate(ctx, dir)
	if err != nil {
		t.Fatalf("GetCommitTemplate error: %v", err)
	}
	if got != "Header\nFooter" {
		t.Errorf("template = %q", got)
	}
}

func TestGetCommitTemplate_FallbackGitmessage(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, ".gitmessage", "feat(scope): subject\n")

	got, err := GetCommitTemplate(ctx, dir)
	if err != nil {
		t.Fatalf("GetCommitTemplate error: %v", err)
	}
	if got != "feat(scope): subject" {
		t.Errorf("template = %q", got)
	}
}

func TestGetCommitTemplate_MissingConfiguredPathErrors(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	runGit(t, dir, "config", "commit.template", "missing-template.txt")

	_, err := GetCommitTemplate(ctx, dir)
	if err == nil || !strings.Contains(err.Error(), "missing-template.txt") {
		t.Fatalf("expected missing configured template error, got %v", err)
	}
}
