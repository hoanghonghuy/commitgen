package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/gitx"
)

// resolveHooksDir returns the absolute path to the repository's git hooks
// directory, honoring repoArg and supporting worktrees/submodules via
// `git rev-parse --git-path hooks`.
func resolveHooksDir(ctx context.Context, repoArg string) (string, error) {
	repoRoot, err := gitx.ResolveRepoRoot(ctx, repoArg)
	if err != nil {
		return "", err
	}

	// `git rev-parse --git-path hooks` resolves the correct hooks directory
	// even for worktrees and submodules (where .git is a file, not a dir).
	out, err := gitx.Git(ctx, repoRoot, "rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", fmt.Errorf("resolve hooks dir: %w", err)
	}
	hooksDir := strings.TrimSpace(out)
	if hooksDir == "" {
		return "", fmt.Errorf("could not determine git hooks directory")
	}
	if !filepath.IsAbs(hooksDir) {
		// git returns the path relative to the repo root.
		hooksDir = filepath.Join(repoRoot, hooksDir)
	}
	return hooksDir, nil
}

// InstallHook installs the prepare-commit-msg hook. When nonInteractive is true
// (or on Windows) the hook runs commitgen in --print mode, which writes the
// message to the commit file without needing /dev/tty.
func InstallHook(ctx context.Context, repoArg string, nonInteractive bool) error {
	useNonInteractive := nonInteractive || runtime.GOOS == "windows"
	if runtime.GOOS == "windows" {
		fmt.Println("Note: On Windows the hook runs in non-interactive (--print) mode and writes the message directly.")
	}

	hooksDir, err := resolveHooksDir(ctx, repoArg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "prepare-commit-msg")

	// Don't overwrite an existing hook blindly.
	if _, err := os.Stat(hookPath); err == nil {
		return fmt.Errorf("hook %s already exists. Please remove it first", hookPath)
	}

	// Resolve the absolute path to the commitgen binary so the hook can call it.
	exe, err := os.Executable()
	if err != nil {
		exe = "commitgen" // fallback: assume it's in PATH
	} else {
		exe, _ = filepath.Abs(exe)
	}

	script := buildHookScript(exe, useNonInteractive)

	if err := os.WriteFile(hookPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write hook file: %w", err)
	}

	fmt.Printf("Hook installed to %s\n", hookPath)
	return nil
}

// buildHookScript returns the prepare-commit-msg shell script. The interactive
// variant uses /dev/tty for the TUI; the non-interactive variant uses --print.
func buildHookScript(exe string, nonInteractive bool) string {
	header := `#!/bin/sh
# commitgen hook
# This hook runs commitgen to generate a commit message.

# $1 is file, $2 is source, $3 is SHA
COMMIT_MSG_FILE=$1
COMMIT_SOURCE=$2
SHA1=$3

# If a message was supplied (e.g. git commit -m), do nothing.
if [ "$COMMIT_SOURCE" = "message" ]; then
  exit 0
fi

echo "commitgen is analyzing changes..."
`
	if nonInteractive {
		// Non-interactive: write the generated message straight to the file.
		return header + fmt.Sprintf("\"%s\" --hook \"$COMMIT_MSG_FILE\" --print\n", exe)
	}
	// Interactive: redirect stdin/stdout to the controlling terminal for the TUI.
	return header + fmt.Sprintf(`if [ -t 0 ]; then
    exec < /dev/tty
fi
"%s" --hook "$COMMIT_MSG_FILE" < /dev/tty > /dev/tty
`, exe)
}

// UninstallHook removes the prepare-commit-msg hook
func UninstallHook(ctx context.Context, repoArg string) error {
	hooksDir, err := resolveHooksDir(ctx, repoArg)
	if err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "prepare-commit-msg")

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		fmt.Println("Hook is not installed.")
		return nil
	}

	if err := os.Remove(hookPath); err != nil {
		return fmt.Errorf("failed to remove hook: %w", err)
	}

	fmt.Println("Hook uninstalled successfully.")
	return nil
}
