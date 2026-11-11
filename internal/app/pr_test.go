package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
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

func TestPromptData_PreservesRenameAndOriginal(t *testing.T) {
	for _, pr := range []bool{false, true} {
		name := "staged"
		if pr {
			name = "pr"
		}
		t.Run(name, func(t *testing.T) {
			dir := initRepo(t)
			original := strings.Repeat("BASE LINE\n", 20)
			writeFile(t, dir, "old.txt", original)
			runGit(t, dir, "add", ".")
			runGit(t, dir, "commit", "-m", "chore: initial")
			runGit(t, dir, "checkout", "-b", "feature/rename")
			runGit(t, dir, "mv", "old.txt", "new.txt")
			writeFile(t, dir, "new.txt", original+"STAGED_ONLY\n")
			runGit(t, dir, "add", "new.txt")
			if pr {
				runGit(t, dir, "commit", "-m", "refactor: rename")
			}
			writeFile(t, dir, "new.txt", "PRIVATE_UNSTAGED\n")
			var data vscodeprompt.Data
			var err error
			if pr {
				data, err = buildPRPromptData(context.Background(), dir, "main", 5, 10, false, "", nil)
			} else {
				data, err = buildPromptData(context.Background(), dir, 5, 10, false, "", nil)
			}
			if err != nil || len(data.Changes) != 1 {
				t.Fatalf("changes=%+v err=%v", data.Changes, err)
			}
			change := data.Changes[0]
			if change.Path != "new.txt" || !strings.Contains(change.Diff, "rename from old.txt") || !strings.Contains(change.Diff, "+STAGED_ONLY") {
				t.Errorf("rename lost: %+v", change)
			}
			if !strings.Contains(change.OriginalCode, "BASE LINE") || strings.Contains(change.OriginalCode, "STAGED_ONLY") || strings.Contains(change.Diff+change.OriginalCode, "PRIVATE_UNSTAGED") {
				t.Errorf("wrong original content: %s", change.OriginalCode)
			}
		})
	}
}

func TestBuildPRPromptData_ExcludesUncommittedContent(t *testing.T) {
	dir := featureBranchRepo(t)
	writeFile(t, dir, "main.go", "package main\n// UNCOMMITTED_PRIVATE_CONTENT\n")
	data, err := buildPRPromptData(context.Background(), dir, "main", 10, 10, false, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.Changes[0].OriginalCode, "func main()") {
		t.Fatal("attachment must contain committed file content")
	}
	if strings.Contains(data.Changes[0].OriginalCode, "UNCOMMITTED_PRIVATE_CONTENT") {
		t.Fatal("attachment includes uncommitted private content")
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

func TestRunPR_EmptyResponseIsError(t *testing.T) {
	for _, response := range []string{"", " \n\t", "```markdown\n\n```"} {
		var err error
		out := captureStdout(t, func() {
			err = runPR(context.Background(), Config{Timeout: time.Second}, "", fakeProvider{resp: response}, vscodeprompt.Data{}, i18n.New(i18n.LocaleEN))
		})
		if err == nil || out != "" {
			t.Errorf("empty PR accepted: err=%v output=%q", err, out)
		}
	}
}
