package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// GetCommitTemplate returns repository commit template content when configured
// or discoverable from common repo-local template paths. Missing templates are
// not errors; unreadable configured templates are returned as errors.
func GetCommitTemplate(ctx context.Context, repoRoot string) (string, error) {
	if configured, err := GitConfig(ctx, repoRoot, "commit.template"); err == nil && strings.TrimSpace(configured) != "" {
		content, err := readTemplatePath(repoRoot, configured)
		if err != nil {
			return "", err
		}
		return content, nil
	}

	for _, candidate := range []string{
		filepath.Join(repoRoot, ".gitmessage"),
		gitPath(ctx, repoRoot, "commit_template"),
	} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		content, err := readTemplatePath(repoRoot, candidate)
		if err == nil {
			return content, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}

	return "", nil
}

func readTemplatePath(repoRoot, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimLeft(path[2:], `/\`))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func gitPath(ctx context.Context, repoRoot, name string) string {
	out, err := Git(ctx, repoRoot, "rev-parse", "--git-path", name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
