package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGit executes a git command in dir during test setup and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// initRepo creates a fresh git repo with a deterministic identity.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "tester@example.com")
	runGit(t, dir, "config", "user.name", "Tester")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "checkout", "-b", "main")
	return dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSplitNonEmptyLines(t *testing.T) {
	in := "a\r\nb\n\n  c  \n"
	got := splitNonEmptyLines(in)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestRepoNameFromRoot(t *testing.T) {
	if got := RepoNameFromRoot(filepath.Join("foo", "bar", "myrepo")); got != "myrepo" {
		t.Errorf("got %q", got)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if !exists(dir) {
		t.Error("existing dir reported missing")
	}
	if exists(filepath.Join(dir, "nope")) {
		t.Error("missing path reported existing")
	}
}

func TestStagedChanges_AndCommitFlow(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	// initial commit so HEAD exists
	writeFile(t, dir, "README.md", "# initial\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "chore: initial commit")

	// stage a modification and a new file
	writeFile(t, dir, "README.md", "# initial\nmore content\n")
	writeFile(t, dir, "new.txt", "brand new\n")
	runGit(t, dir, "add", "README.md", "new.txt")

	files, err := StagedFileNames(ctx, dir)
	if err != nil {
		t.Fatalf("StagedFileNames error: %v", err)
	}
	if len(files) != 2 || files[0] != "README.md" || files[1] != "new.txt" {
		t.Fatalf("StagedFileNames = %v", files)
	}

	changes, err := StagedChanges(ctx, dir, 10)
	if err != nil {
		t.Fatalf("StagedChanges error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 staged changes, got %d: %+v", len(changes), changes)
	}
	for _, ch := range changes {
		if ch.Diff == "" {
			t.Errorf("expected non-empty diff for %s", ch.Path)
		}
	}

	// OriginalFileAtHEAD returns prior content of README
	orig, err := OriginalFileAtHEAD(ctx, dir, "README.md")
	if err != nil {
		t.Fatalf("OriginalFileAtHEAD error: %v", err)
	}
	if orig != "# initial\n" {
		t.Errorf("unexpected HEAD content: %q", orig)
	}

	// new.txt does not exist at HEAD → error
	if _, err := OriginalFileAtHEAD(ctx, dir, "new.txt"); err == nil {
		t.Error("expected error reading new file at HEAD")
	}

	// ReadWorkingTreeFile reads current content
	wt, err := ReadWorkingTreeFile(dir, "new.txt")
	if err != nil || wt != "brand new\n" {
		t.Errorf("ReadWorkingTreeFile = %q, err=%v", wt, err)
	}
}

func TestStagedChanges_LimitAndEmpty(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "base.txt", "x\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// no staged changes now
	changes, err := StagedChanges(ctx, dir, 10)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 staged changes, got %d", len(changes))
	}

	// stage 3 files, limit to 2
	writeFile(t, dir, "a.txt", "a\n")
	writeFile(t, dir, "b.txt", "b\n")
	writeFile(t, dir, "c.txt", "c\n")
	runGit(t, dir, "add", ".")
	limited, err := StagedChanges(ctx, dir, 2)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected limit of 2, got %d", len(limited))
	}
}

func TestCurrentBranchAndConfigAndCommits(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "f.txt", "1\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: first")
	writeFile(t, dir, "f.txt", "2\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "fix: second")

	branch, err := CurrentBranch(ctx, dir)
	if err != nil || branch != "main" {
		t.Errorf("CurrentBranch = %q, err=%v", branch, err)
	}

	email, err := GitConfig(ctx, dir, "user.email")
	if err != nil || email != "tester@example.com" {
		t.Errorf("GitConfig = %q, err=%v", email, err)
	}

	commits, err := RecentCommits(ctx, dir, 5)
	if err != nil {
		t.Fatalf("RecentCommits error: %v", err)
	}
	if len(commits) != 2 || commits[0] != "fix: second" {
		t.Errorf("RecentCommits = %v", commits)
	}

	// n <= 0 returns nil
	if c, _ := RecentCommits(ctx, dir, 0); c != nil {
		t.Errorf("expected nil for n=0, got %v", c)
	}

	byAuthor, err := RecentCommitsByAuthor(ctx, dir, 5, "tester@example.com")
	if err != nil {
		t.Fatalf("RecentCommitsByAuthor error: %v", err)
	}
	if len(byAuthor) != 2 {
		t.Errorf("RecentCommitsByAuthor = %v", byAuthor)
	}

	// empty author returns nil
	if c, _ := RecentCommitsByAuthor(ctx, dir, 5, "  "); c != nil {
		t.Errorf("expected nil for empty author, got %v", c)
	}
}

func TestCommit(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	// empty message rejected
	if err := Commit(ctx, dir, "   "); err == nil {
		t.Error("expected error for empty commit message")
	}

	writeFile(t, dir, "f.txt", "data\n")
	runGit(t, dir, "add", ".")
	if err := Commit(ctx, dir, "feat: add f"); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	commits, _ := RecentCommits(ctx, dir, 1)
	if len(commits) != 1 || commits[0] != "feat: add f" {
		t.Errorf("commit not recorded: %v", commits)
	}
}

func TestResolveRepoRoot(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "f.txt", "x\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	// resolve from explicit repo arg pointing at a subdir
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := ResolveRepoRoot(ctx, sub)
	if err != nil {
		t.Fatalf("ResolveRepoRoot error: %v", err)
	}
	// git returns the toplevel; compare resolved real paths
	wantRoot, _ := filepath.EvalSymlinks(dir)
	gotRoot, _ := filepath.EvalSymlinks(root)
	if gotRoot != wantRoot {
		t.Errorf("ResolveRepoRoot = %q; want %q", gotRoot, wantRoot)
	}

	// non-existent repo arg errors
	if _, err := ResolveRepoRoot(ctx, filepath.Join(dir, "missing-dir")); err == nil {
		t.Error("expected error for non-existent repo arg")
	}
}

func TestGit_ErrorIncludesStderr(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	_, err := Git(ctx, dir, "this-is-not-a-git-command")
	if err == nil {
		t.Error("expected error for invalid git subcommand")
	}
}
