package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func ResolveRepoRoot(repoArg string) (string, error) {
	if strings.TrimSpace(repoArg) != "" {
		p, err := filepath.Abs(repoArg)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		// If user points to subdir, normalize by asking git
		root, err := Git(p, "rev-parse", "--show-toplevel")
		if err == nil {
			return strings.TrimSpace(root), nil
		}
		return p, nil
	}

	// Walk up from current dir to find repo using git itself (best for worktrees/submodules).
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// try git directly from cwd
	root, err := Git(cwd, "rev-parse", "--show-toplevel")
	if err == nil {
		return strings.TrimSpace(root), nil
	}

	// fallback: walk up to find .git (works for normal repos; not perfect for all worktrees)
	cur := cwd
	for {
		if exists(filepath.Join(cur, ".git")) {
			// confirm via git
			root, err := Git(cur, "rev-parse", "--show-toplevel")
			if err == nil {
				return strings.TrimSpace(root), nil
			}
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return "", errors.New("not inside a git repository. Use --repo /path/to/repo")
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func RepoNameFromRoot(repoRoot string) string {
	return filepath.Base(repoRoot)
}
