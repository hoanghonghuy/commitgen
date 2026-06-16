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
// message to the commit file without needing /dev/tty. configPath, when set,
// is forwarded to the hook so a custom config location is honored at commit time.
func InstallHook(ctx context.Context, repoArg string, nonInteractive bool, configPath string) error {
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

	// Back up an existing hook instead of failing, so a pre-existing hook is
	// never silently lost. The user can restore it from the .bak file.
	if _, err := os.Stat(hookPath); err == nil {
		backupPath := hookPath + ".bak"
		if err := os.Rename(hookPath, backupPath); err != nil {
			return fmt.Errorf("back up existing hook %s: %w", hookPath, err)
		}
		fmt.Printf("Existing hook backed up to %s\n", backupPath)
	}

	// Resolve the absolute path to the commitgen binary so the hook can call it.
	exe, err := os.Executable()
	if err != nil {
		exe = "commitgen" // fallback: assume it's in PATH
	} else {
		exe, _ = filepath.Abs(exe)
	}

	configArg := ""
	if strings.TrimSpace(configPath) != "" {
		if abs, absErr := filepath.Abs(configPath); absErr == nil {
			configPath = abs
		}
		configArg = fmt.Sprintf(" --config \"%s\"", configPath)
	}

	script := buildHookScript(exe, useNonInteractive, configArg)

	if err := os.WriteFile(hookPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write hook file: %w", err)
	}

	fmt.Printf("Hook installed to %s\n", hookPath)
	return nil
}

// buildHookScript returns the prepare-commit-msg shell script. The interactive
// variant uses /dev/tty for the TUI; the non-interactive variant uses --print.
// configArg is an optional pre-formatted ` --config "<path>"` fragment.
func buildHookScript(exe string, nonInteractive bool, configArg string) string {
	header := `#!/bin/sh
# commitgen hook
# This hook runs commitgen to generate a commit message.

# $1 is file, $2 is source, $3 is SHA
COMMIT_MSG_FILE=$1
COMMIT_SOURCE=$2
SHA1=$3

# Skip when the message is already provided or managed by git:
#   message       -> git commit -m / -F
#   merge/squash  -> merge or squash commit messages
#   commit        -> git commit --amend / -c / -C (reusing an existing message)
case "$COMMIT_SOURCE" in
  message|merge|squash|commit)
    exit 0
    ;;
esac

echo "commitgen is analyzing changes..."
`
	if nonInteractive {
		// Non-interactive: write the generated message straight to the file.
		// Abort the commit if commitgen fails so git never commits an empty or
		// stale message.
		return header + fmt.Sprintf(`if ! "%s" --hook "$COMMIT_MSG_FILE" --print%s; then
  echo "commitgen failed, aborting commit" >&2
  exit 1
fi
`, exe, configArg)
	}
	// Interactive: redirect stdin/stdout to the controlling terminal for the TUI.
	// Abort the commit when commitgen exits non-zero.
	return header + fmt.Sprintf(`if ! "%s" --hook "$COMMIT_MSG_FILE"%s < /dev/tty > /dev/tty; then
  echo "commitgen failed, aborting commit" >&2
  exit 1
fi
`, exe, configArg)
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
