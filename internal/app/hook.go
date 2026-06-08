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

// InstallHook installs the prepare-commit-msg hook
func InstallHook(ctx context.Context, repoArg string) error {
	if runtime.GOOS == "windows" {
		fmt.Println("Warning: The git hook uses /dev/tty and #!/bin/sh which may not work correctly on Windows.")
		fmt.Println("Consider running commitgen manually instead of using the hook on Windows.")
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

	script := fmt.Sprintf(`#!/bin/sh
# commitgen hook
# This hook runs commitgen to generate a commit message.
# It uses /dev/tty to allow interaction even inside a hook.

# Only run if no message is given (e.g. not a merge, not --amend with message)
# $1 is file, $2 is source, $3 is SHA

COMMIT_MSG_FILE=$1
COMMIT_SOURCE=$2
SHA1=$3

# Skip if amending or if message source is arguably "template" or "message" provided?
# Usually we want it for empty "git commit".
# If source is "message" (-m), skip.
if [ "$COMMIT_SOURCE" = "message" ]; then
  exit 0
fi

# Run commitgen in hook mode
# We redirect stdin/stdout to tty to allow interactive UI
if [ -t 0 ]; then
    exec < /dev/tty
fi

echo "commitgen is analyzing changes..."
"%s" --hook "$COMMIT_MSG_FILE" < /dev/tty > /dev/tty

# If commitgen succeeds, it writes to the file.
`, exe)

	if err := os.WriteFile(hookPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write hook file: %w", err)
	}

	fmt.Printf("Hook installed to %s\n", hookPath)
	return nil
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
