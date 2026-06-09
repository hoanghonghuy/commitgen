package gitx

import (
	"context"
	"path/filepath"
	"testing"
)

func TestResolveRepoRoot_FromCwd(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "f.txt", "x\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// Change into the repo and resolve with empty arg (cwd path).
	t.Chdir(dir)
	root, err := ResolveRepoRoot(ctx, "")
	if err != nil {
		t.Fatalf("ResolveRepoRoot(cwd) error: %v", err)
	}
	wantRoot, _ := filepath.EvalSymlinks(dir)
	gotRoot, _ := filepath.EvalSymlinks(root)
	if gotRoot != wantRoot {
		t.Errorf("ResolveRepoRoot = %q; want %q", gotRoot, wantRoot)
	}
}

func TestResolveRepoRoot_FromSubdirCwd(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "f.txt", "x\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	sub := filepath.Join(dir, "a", "b")
	writeFile(t, dir, "a/b/keep.txt", "y\n")
	t.Chdir(sub)

	root, err := ResolveRepoRoot(ctx, "")
	if err != nil {
		t.Fatalf("ResolveRepoRoot(subdir cwd) error: %v", err)
	}
	wantRoot, _ := filepath.EvalSymlinks(dir)
	gotRoot, _ := filepath.EvalSymlinks(root)
	if gotRoot != wantRoot {
		t.Errorf("ResolveRepoRoot = %q; want %q", gotRoot, wantRoot)
	}
}

func TestResolveRepoRoot_NotInRepo(t *testing.T) {
	ctx := context.Background()
	// A temp dir that is not under any git repo.
	dir := t.TempDir()
	t.Chdir(dir)
	if _, err := ResolveRepoRoot(ctx, ""); err == nil {
		t.Error("expected error when cwd is not in a git repository")
	}
}

func TestOriginalFileAtHEAD_NewFileSilent(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "base.txt", "1\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// A file that does not exist at HEAD should return an error (handled silently in logs).
	if _, err := OriginalFileAtHEAD(ctx, dir, "brand-new.txt"); err == nil {
		t.Error("expected error for file not in HEAD")
	}
}

func TestReadWorkingTreeFile_Missing(t *testing.T) {
	dir := initRepo(t)
	if _, err := ReadWorkingTreeFile(dir, "no-such-file.txt"); err == nil {
		t.Error("expected error reading missing working tree file")
	}
}

func TestCommitAmend(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "f.txt", "1\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: original message")

	// Stage another change and amend.
	writeFile(t, dir, "f.txt", "2\n")
	runGit(t, dir, "add", ".")
	if err := CommitAmend(ctx, dir, "feat: amended message"); err != nil {
		t.Fatalf("CommitAmend error: %v", err)
	}

	commits, _ := RecentCommits(ctx, dir, 5)
	if len(commits) != 1 {
		t.Errorf("amend should not add a commit, got %d commits: %v", len(commits), commits)
	}
	if commits[0] != "feat: amended message" {
		t.Errorf("amended message = %q", commits[0])
	}

	// Empty message is rejected.
	if err := CommitAmend(ctx, dir, "  "); err == nil {
		t.Error("expected error for empty amend message")
	}
}
