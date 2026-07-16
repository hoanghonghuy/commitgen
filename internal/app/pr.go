package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// buildPRPromptData gathers commits and file diffs from merge-base(base, HEAD)..HEAD.
func buildPRPromptData(ctx context.Context, repoRoot, basePreferred string, recentN, maxFiles int, summarize bool, customInstructions string, ignoredFiles []string) (vscodeprompt.Data, error) {
	recentN = clampNonNegative(recentN)
	if maxFiles <= 0 {
		maxFiles = 10
	}

	baseRef, err := gitx.ResolveBaseRef(ctx, repoRoot, basePreferred)
	if err != nil {
		return vscodeprompt.Data{}, logger.LogError(err, "failed to resolve PR base branch")
	}

	mb, err := gitx.MergeBase(ctx, repoRoot, baseRef, "HEAD")
	if err != nil {
		return vscodeprompt.Data{}, logger.LogError(err, "failed to compute merge-base", "base", baseRef)
	}

	branch, _ := gitx.CurrentBranch(ctx, repoRoot)
	commits, _ := gitx.CommitsBetween(ctx, repoRoot, mb, "HEAD", recentN)
	files, err := gitx.ChangedFiles(ctx, repoRoot, mb, "HEAD")
	if err != nil {
		return vscodeprompt.Data{}, logger.LogError(err, "failed to list changed files since merge-base")
	}

	defaultIgnores := []string{
		"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"*.map", "*.svg", "*.min.js", "*.min.css",
	}
	allIgnores := append(defaultIgnores, ignoredFiles...)

	const maxDiffSize = 100 * 1024
	filtered := make([]vscodeprompt.Change, 0, maxFiles)
	for _, path := range files {
		if len(filtered) >= maxFiles {
			break
		}
		if shouldIgnore(path, allIgnores) {
			continue
		}
		diff, _ := gitx.DiffFile(ctx, repoRoot, mb, "HEAD", path)
		ch := vscodeprompt.Change{Path: path, Diff: diff}
		if len(ch.Diff) > maxDiffSize {
			ch.Diff = truncateUTF8(ch.Diff, 2000) + "\n...[Diff truncated due to size]..."
		}

		orig, _ := gitx.FileAtRef(ctx, repoRoot, mb, path)
		if strings.TrimSpace(orig) == "" {
			orig, _ = gitx.ReadWorkingTreeFile(repoRoot, path)
		}
		if len(orig) > maxDiffSize {
			orig = truncateUTF8(orig, 2000) + "\n...[Content truncated due to size]..."
		}
		attachment := vscodeprompt.BuildAttachment(repoRoot, path, orig, summarize)
		filtered = append(filtered, vscodeprompt.Change{
			Path:         path,
			Diff:         ch.Diff,
			OriginalCode: attachment,
		})
	}

	if len(commits) == 0 && len(filtered) == 0 {
		return vscodeprompt.Data{}, ErrNoPRChanges
	}

	return vscodeprompt.Data{
		RepositoryName:       gitx.RepoNameFromRoot(repoRoot),
		BranchName:           branch,
		BaseBranch:           baseRef,
		RecentRepoCommits:    commits,
		Changes:              filtered,
		CustomInstructions:   customInstructions,
		SummarizeAttachments: summarize,
	}, nil
}

// runPR generates a PR title and description and prints them to stdout.
func runPR(ctx context.Context, cfg Config, repoRoot string, provider ai.Provider, data vscodeprompt.Data, tr *i18n.Translator) error {
	msgs := vscodeprompt.BuildPRMessages(data)

	cctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	raw, err := provider.Generate(cctx, msgs, clampTemperature(cfg.Temperature))
	if err != nil {
		return logger.LogError(err, "failed to generate PR description")
	}

	title, body, ok := vscodeprompt.ParsePROutput(raw)
	if !ok {
		fmt.Println(strings.TrimSpace(raw))
		return nil
	}

	fmt.Printf("# %s\n", title)
	if body != "" {
		fmt.Printf("\n%s\n", body)
	}
	return nil
}
