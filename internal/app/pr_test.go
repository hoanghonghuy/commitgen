package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

func featureBranchRepo(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# init\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	runGit(t, dir, "checkout", "-b", "feature/pr")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: add main")
	return dir
}

func TestBuildPRPromptData(t *testing.T) {
	ctx := context.Background()
	dir := featureBranchRepo(t)

	data, err := buildPRPromptData(ctx, dir, "main", 10, 10, false, "", nil)
	if err != nil {
		t.Fatalf("buildPRPromptData: %v", err)
	}
	if data.BaseBranch != "main" || data.BranchName != "feature/pr" {
		t.Fatalf("branches: base=%q head=%q", data.BaseBranch, data.BranchName)
	}
	if len(data.RecentRepoCommits) == 0 {
		t.Fatal("expected commits since base")
	}
	if len(data.Changes) != 1 || data.Changes[0].Path != "main.go" {
		t.Fatalf("changes: %+v", data.Changes)
	}
}

func TestBuildPRPromptData_NoChanges(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "# init\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: init")

	_, err := buildPRPromptData(ctx, dir, "main", 10, 10, false, "", nil)
	if err == nil {
		t.Fatal("expected error when no commits/changes since base")
	}
}

func TestRunPR(t *testing.T) {
	dir := featureBranchRepo(t)
	tr := i18n.New(i18n.LocaleEN)
	cfg := Config{
		Command:     "pr",
		BaseBranch:  "main",
		Timeout:     5 * time.Second,
		Temperature: 0.2,
	}
	data, err := buildPRPromptData(context.Background(), dir, "main", 10, 10, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := "```markdown\n# feat: add main entrypoint\n\n## Summary\n- Add main.go\n\n## Test plan\n- [ ] go run .\n```"
	out := captureStdout(t, func() {
		if err := runPR(context.Background(), cfg, dir, fakeProvider{resp: resp}, data, tr); err != nil {
			t.Fatalf("runPR: %v", err)
		}
	})
	if !strings.Contains(out, "feat: add main entrypoint") {
		t.Errorf("output missing title: %q", out)
	}
	if !strings.Contains(out, "## Summary") {
		t.Errorf("output missing body: %q", out)
	}
}
