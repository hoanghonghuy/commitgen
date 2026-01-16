package gitx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type StagedChange struct {
	Path string
	Diff string
}

func Git(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v failed: %v\n%s", args, err, stderr.String())
	}
	return stdout.String(), nil
}

func CurrentBranch(repoRoot string) (string, error) {
	out, err := Git(repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func GitConfig(repoRoot, key string) (string, error) {
	out, err := Git(repoRoot, "config", "--get", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func RecentCommits(repoRoot string, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	out, err := Git(repoRoot, "log", fmt.Sprintf("-n%d", n), "--pretty=format:%s")
	if err != nil {
		return nil, err
	}
	return splitNonEmptyLines(out), nil
}

func RecentCommitsByAuthor(repoRoot string, n int, author string) ([]string, error) {
	if n <= 0 || strings.TrimSpace(author) == "" {
		return nil, nil
	}
	out, err := Git(repoRoot, "log", fmt.Sprintf("-n%d", n), fmt.Sprintf("--author=%s", author), "--pretty=format:%s")
	if err != nil {
		return nil, err
	}
	return splitNonEmptyLines(out), nil
}

func StagedChanges(repoRoot string, maxFiles int) ([]StagedChange, error) {
	if maxFiles <= 0 {
		maxFiles = 10
	}
	filesOut, err := Git(repoRoot, "diff", "--staged", "--name-only")
	if err != nil {
		return nil, err
	}
	files := splitNonEmptyLines(filesOut)
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	var out []StagedChange
	for _, f := range files {
		diff, _ := Git(repoRoot, "diff", "--staged", "--", f)
		out = append(out, StagedChange{Path: f, Diff: diff})
	}
	return out, nil
}

func OriginalFileAtHEAD(repoRoot, relPath string) (string, error) {
	spec := "HEAD:" + relPath
	out, err := Git(repoRoot, "show", spec)
	if err != nil {
		return "", err
	}
	return out, nil
}

func ReadWorkingTreeFile(repoRoot, relPath string) (string, error) {
	p := filepath.Join(repoRoot, relPath)
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Commit(repoRoot, message string) error {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return fmt.Errorf("commit message cannot be empty")
	}
	// Use -m to commit
	_, err := Git(repoRoot, "commit", "-m", msg)
	if err != nil {
		return err
	}
	// Maybe print success?
	fmt.Println("Commit successful!")
	return nil
}

func splitNonEmptyLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}
