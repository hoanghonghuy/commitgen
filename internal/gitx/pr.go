package gitx

import (
	"context"
	"fmt"
	"strings"
)

// MergeBase returns the merge-base SHA of the two commits/refs.
func MergeBase(ctx context.Context, repoRoot, a, b string) (string, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", fmt.Errorf("merge-base requires two non-empty refs")
	}
	out, err := Git(ctx, repoRoot, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CommitsBetween returns commit subjects on the path from exclusive start
// (typically a merge-base) to inclusive end (typically HEAD), newest first.
func CommitsBetween(ctx context.Context, repoRoot, startExclusive, endInclusive string, n int) ([]string, error) {
	startExclusive = strings.TrimSpace(startExclusive)
	endInclusive = strings.TrimSpace(endInclusive)
	if startExclusive == "" || endInclusive == "" {
		return nil, nil
	}
	args := []string{"log", startExclusive + ".." + endInclusive, "--pretty=format:%s"}
	if n > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", n))
	}
	out, err := Git(ctx, repoRoot, args...)
	if err != nil {
		return nil, err
	}
	return splitNonEmptyLines(out), nil
}

// ChangedFiles returns paths that differ between start and end (three-dot style
// when start is a merge-base: git diff --name-only start...end).
func ChangedFiles(ctx context.Context, repoRoot, start, end string) ([]string, error) {
	changes, err := ChangedFileChanges(ctx, repoRoot, start, end)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	return paths, nil
}

// ChangedFileChanges lists committed changes with their original paths.
func ChangedFileChanges(ctx context.Context, repoRoot, start, end string) ([]FileChange, error) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" || end == "" {
		return nil, fmt.Errorf("ChangedFiles requires start and end refs")
	}
	out, err := Git(ctx, repoRoot, "diff", "--find-renames", "--raw", "-z", start+"..."+end)
	if err != nil {
		return nil, err
	}
	return parseFileChanges(out)
}

// DiffFile returns the unified diff for a single path between start and end.
func DiffFile(ctx context.Context, repoRoot, start, end, path string, originalPaths ...string) (string, error) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" || end == "" || path == "" {
		return "", fmt.Errorf("DiffFile requires start, end, and path")
	}
	return fileDiff(ctx, repoRoot, []string{start + "..." + end}, path, originalPaths)
}

// ResolveBaseRef returns an explicit base when provided, otherwise tries
// common default branch names that exist in the repository.
func ResolveBaseRef(ctx context.Context, repoRoot, preferred string) (string, error) {
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		if err := ensureRevExists(ctx, repoRoot, preferred); err != nil {
			return "", fmt.Errorf("base ref %q not found: %w", preferred, err)
		}
		return preferred, nil
	}
	for _, candidate := range []string{"main", "master", "develop", "trunk"} {
		if err := ensureRevExists(ctx, repoRoot, candidate); err == nil {
			return candidate, nil
		}
	}
	// Fall back to upstream of current branch when set.
	if up, err := Git(ctx, repoRoot, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		up = strings.TrimSpace(up)
		if up != "" {
			return up, nil
		}
	}
	return "", fmt.Errorf("could not resolve a base branch (tried main, master, develop, trunk, and upstream)")
}

func ensureRevExists(ctx context.Context, repoRoot, rev string) error {
	_, err := Git(ctx, repoRoot, "rev-parse", "--verify", rev)
	return err
}

// FileAtRef returns file content at a given git ref (e.g. merge-base SHA).
func FileAtRef(ctx context.Context, repoRoot, ref, relPath string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || relPath == "" {
		return "", fmt.Errorf("FileAtRef requires ref and path")
	}
	out, err := Git(ctx, repoRoot, "show", ref+":"+relPath)
	if err != nil {
		return "", err
	}
	return out, nil
}
