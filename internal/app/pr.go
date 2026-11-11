package app

import (
	"context"
	"fmt"

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
	files, err := gitx.ChangedFileChanges(ctx, repoRoot, mb, "HEAD")
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
	for _, file := range files {
		path := file.Path
		if len(filtered) >= maxFiles {
			break
		}
		if shouldIgnore(path, allIgnores) {
			continue
		}
		diff, err := gitx.DiffFile(ctx, repoRoot, mb, "HEAD", path, file.OriginalPath)
		if err != nil {
			return vscodeprompt.Data{}, err
		}
		ch := vscodeprompt.Change{Path: path, Diff: diff}
		if len(ch.Diff) > maxDiffSize {
			ch.Diff = truncateUTF8(ch.Diff, 2000) + "\n...[Diff truncated due to size]..."
		}

		originalPath := file.OriginalPath
		if file.IsSubmodule {
			filtered = append(filtered, ch)
			continue
		}
		ref := mb
		if originalPath == "" {
			originalPath = path
			ref = "HEAD"
		}
		orig, err := gitx.FileAtRef(ctx, repoRoot, ref, originalPath)
		if err != nil {
			return vscodeprompt.Data{}, err
		}
		if len(orig) > maxDiffSize {
			orig = truncateUTF8(orig, 2000) + "\n...[Content truncated due to size]..."
		}
		attachment := vscodeprompt.BuildAttachment(repoRoot, originalPath, orig, summarize)
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

	content, err := extractGeneratedText(raw, "PR description")
	if err != nil {
		return logger.LogError(err, "failed to generate PR description")
	}
	title, body, ok := vscodeprompt.ParsePROutput(content)
	if !ok {
		fmt.Println(content)
		return nil
	}

	fmt.Printf("# %s\n", title)
	if body != "" {
		fmt.Printf("\n%s\n", body)
	}
	return nil
}
