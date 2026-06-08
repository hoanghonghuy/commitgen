package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// fakeProvider is a test double implementing ai.Provider.
type fakeProvider struct {
	resp string
	err  error
}

func (f fakeProvider) Generate(_ context.Context, _ []vscodeprompt.VSCodeMessage, _ float64) (string, error) {
	return f.resp, f.err
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

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
