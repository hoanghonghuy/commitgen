package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

func TestResolveHooksDir(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	hooksDir, err := resolveHooksDir(ctx, dir)
	if err != nil {
		t.Fatalf("resolveHooksDir error: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(hooksDir), ".git/hooks") {
		t.Errorf("unexpected hooks dir: %q", hooksDir)
	}
}

func TestResolveHooksDir_NotARepo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() // not a git repo
	if _, err := resolveHooksDir(ctx, dir); err == nil {
		t.Error("expected error resolving hooks dir outside a repo")
	}
}

func TestInstallAndUninstallHook(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)

	tr := i18n.New(i18n.LocaleEN)
	if err := InstallHook(ctx, dir, false, "", tr); err != nil {
		t.Fatalf("InstallHook error: %v", err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "prepare-commit-msg")
	b, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook file not created: %v", err)
	}
	if !strings.Contains(string(b), "commitgen") {
		t.Error("hook script should reference commitgen")
	}
	if !strings.Contains(string(b), "--print") {
		t.Error("hook script should always use --print headless mode")
	}

	// Installing again should back up the existing hook, not fail.
	if err := InstallHook(ctx, dir, false, "", tr); err != nil {
		t.Errorf("InstallHook over existing hook should back up, not error: %v", err)
	}
	if _, err := os.Stat(hookPath + ".bak"); err != nil {
		t.Errorf("expected backup hook at %s.bak: %v", hookPath, err)
	}

	// Uninstall removes the active hook and restores the backed-up hook when present.
	if err := UninstallHook(ctx, dir, tr); err != nil {
		t.Fatalf("UninstallHook error: %v", err)
	}
	if _, err := os.Stat(hookPath + ".bak"); err == nil {
		t.Error("backup should be consumed after restore")
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook should be restored from backup: %v", err)
	}

	// Uninstall when absent is a no-op (no error)
	if err := UninstallHook(ctx, dir, tr); err != nil {
		t.Errorf("UninstallHook on absent hook should not error: %v", err)
	}
}
