package gitx

import (
	"context"
	"strings"
	"testing"
)

func TestMergeBase_AndCommitsBetween(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "app.go", "package main\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: add app")
	writeFile(t, dir, "app.go", "package main\n\nfunc main() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: add main")

	mb, err := MergeBase(ctx, dir, "main", "HEAD")
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if mb == "" {
		t.Fatal("expected non-empty merge-base")
	}

	commits, err := CommitsBetween(ctx, dir, mb, "HEAD", 10)
	if err != nil {
		t.Fatalf("CommitsBetween: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits since merge-base, got %v", commits)
	}
	if !strings.Contains(commits[0], "feat:") {
		t.Errorf("unexpected commits: %v", commits)
	}
}

func TestChangedFilesSinceMergeBase(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "src/a.go", "package a\n")
	writeFile(t, dir, "src/b.go", "package b\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: add packages")

	mb, err := MergeBase(ctx, dir, "main", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	files, err := ChangedFiles(ctx, dir, mb, "HEAD")
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %v", files)
	}

	diff, err := DiffFile(ctx, dir, mb, "HEAD", "src/a.go")
	if err != nil {
		t.Fatalf("DiffFile: %v", err)
	}
	if !strings.Contains(diff, "package a") {
		t.Errorf("diff missing content: %q", diff)
	}

	content, err := FileAtRef(ctx, dir, "HEAD", "src/a.go")
	if err != nil {
		t.Fatalf("FileAtRef: %v", err)
	}
	if content != "package a\n" {
		t.Errorf("FileAtRef content=%q", content)
	}
	if _, err := FileAtRef(ctx, dir, "", "src/a.go"); err == nil {
		t.Error("expected FileAtRef error for empty ref")
	}
	if _, err := FileAtRef(ctx, dir, "HEAD", ""); err == nil {
		t.Error("expected FileAtRef error for empty path")
	}
}

func TestResolveBaseRef(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# i\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	got, err := ResolveBaseRef(ctx, dir, "main")
	if err != nil || got != "main" {
		t.Fatalf("explicit base: got %q err=%v", got, err)
	}

	got, err = ResolveBaseRef(ctx, dir, "")
	if err != nil || got != "main" {
		t.Fatalf("auto base: got %q err=%v", got, err)
	}
}
