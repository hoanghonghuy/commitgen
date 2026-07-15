package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/gitx"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

// resolveHooksDir returns the absolute path to the repository's git hooks
// directory, honoring repoArg and supporting worktrees/submodules via
// `git rev-parse --git-path hooks`.
func resolveHooksDir(ctx context.Context, repoArg string) (string, error) {
	repoRoot, err := gitx.ResolveRepoRoot(ctx, repoArg)
	if err != nil {
		return "", err
	}

	out, err := gitx.Git(ctx, repoRoot, "rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", fmt.Errorf("resolve hooks dir: %w", err)
	}
	hooksDir := strings.TrimSpace(out)
	if hooksDir == "" {
		return "", fmt.Errorf("could not determine git hooks directory")
	}
	if !filepath.IsAbs(hooksDir) {
		hooksDir = filepath.Join(repoRoot, hooksDir)
	}
	return hooksDir, nil
}

// InstallHook installs the prepare-commit-msg hook. The hook always runs
// commitgen in headless --print mode so git never opens the alternate-screen TUI.
// configPath, when set, is forwarded to the hook so a custom config location is
// honored at commit time.
func InstallHook(ctx context.Context, repoArg string, _ bool, configPath string, tr *i18n.Translator) error {
	hooksDir, err := resolveHooksDir(ctx, repoArg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "prepare-commit-msg")

	if _, err := os.Stat(hookPath); err == nil {
		backupPath := hookPath + ".bak"
		if err := os.Rename(hookPath, backupPath); err != nil {
			return fmt.Errorf("back up existing hook %s: %w", hookPath, err)
		}
		fmt.Println(tr.T("hook.backed_up", backupPath))
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "commitgen"
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

	script := buildHookScript(exe, configArg, tr.T("hook.analyzing"), tr.T("hook.failed"))

	if err := os.WriteFile(hookPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write hook file: %w", err)
	}

	fmt.Println(tr.T("hook.installed", hookPath))
	return nil
}

// buildHookScript returns the prepare-commit-msg shell script. It always uses
// --print so the hook never launches the TUI inside git.
func buildHookScript(exe, configArg, analyzingMsg, failedMsg string) string {
	return fmt.Sprintf(`#!/bin/sh
# commitgen hook
# This hook runs commitgen to generate a commit message.

COMMIT_MSG_FILE=$1
COMMIT_SOURCE=$2
SHA1=$3

case "$COMMIT_SOURCE" in
  message|merge|squash|commit)
    exit 0
    ;;
esac

echo "%s"
if ! "%s" --hook "$COMMIT_MSG_FILE" --print%s; then
  echo "%s" >&2
  exit 1
fi
`, analyzingMsg, exe, configArg, failedMsg)
}

// UninstallHook removes the prepare-commit-msg hook. When a .bak file exists
// (from a prior install), it is restored as the active hook.
func UninstallHook(ctx context.Context, repoArg string, tr *i18n.Translator) error {
	hooksDir, err := resolveHooksDir(ctx, repoArg)
	if err != nil {
		return err
	}

	hookPath := filepath.Join(hooksDir, "prepare-commit-msg")
	backupPath := hookPath + ".bak"

	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		if _, berr := os.Stat(backupPath); berr == nil {
			if err := os.Rename(backupPath, hookPath); err != nil {
				return fmt.Errorf("restore hook from backup: %w", err)
			}
			fmt.Println(tr.T("hook.restored_backup", hookPath))
			return nil
		}
		fmt.Println(tr.T("hook.not_installed"))
		return nil
	}

	if err := os.Remove(hookPath); err != nil {
		return fmt.Errorf("failed to remove hook: %w", err)
	}

	if _, err := os.Stat(backupPath); err == nil {
		if err := os.Rename(backupPath, hookPath); err != nil {
			return fmt.Errorf("restore hook from backup: %w", err)
		}
		fmt.Println(tr.T("hook.restored_backup", hookPath))
		return nil
	}

	fmt.Println(tr.T("hook.uninstalled"))
	return nil
}
