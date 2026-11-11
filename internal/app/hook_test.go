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
	originalHook := "#!/bin/sh\n# original prepare hook\nexit 0\n"
	writeFile(t, dir, ".git/hooks/prepare-commit-msg", originalHook)
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
	if restored, err := os.ReadFile(hookPath); err != nil || string(restored) != originalHook {
		t.Fatalf("original hook not restored: content=%q err=%v", restored, err)
	}

	// Repeating uninstall must leave the restored user hook untouched.
	if err := UninstallHook(ctx, dir, tr); err != nil {
		t.Errorf("UninstallHook on absent hook should not error: %v", err)
	}
	if restored, err := os.ReadFile(hookPath); err != nil || string(restored) != originalHook {
		t.Fatalf("repeated uninstall removed user hook: content=%q err=%v", restored, err)
	}
}

func TestInstallAndUninstallMsgHook(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	tr := i18n.New(i18n.LocaleEN)
	originalHook := "#!/bin/sh\n# original message hook\nexit 0\n"
	writeFile(t, dir, ".git/hooks/commit-msg", originalHook)

	if err := InstallMsgHook(ctx, dir, "", tr); err != nil {
		t.Fatalf("InstallMsgHook error: %v", err)
	}
	hookPath := filepath.Join(dir, ".git", "hooks", "commit-msg")
	b, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("commit-msg hook file not created: %v", err)
	}
	script := string(b)
	for _, want := range []string{"validate-msg", "--file", "COMMIT_MSG_FILE"} {
		if !strings.Contains(script, want) {
			t.Errorf("commit-msg hook missing %q:\n%s", want, script)
		}
	}

	if err := InstallMsgHook(ctx, dir, "", tr); err != nil {
		t.Errorf("InstallMsgHook over existing hook should back up, not error: %v", err)
	}
	if _, err := os.Stat(hookPath + ".bak"); err != nil {
		t.Errorf("expected backup commit-msg hook at %s.bak: %v", hookPath, err)
	}

	if err := UninstallMsgHook(ctx, dir, tr); err != nil {
		t.Fatalf("UninstallMsgHook error: %v", err)
	}
	if _, err := os.Stat(hookPath + ".bak"); err == nil {
		t.Error("backup should be consumed after commit-msg restore")
	}
	if restored, err := os.ReadFile(hookPath); err != nil || string(restored) != originalHook {
		t.Fatalf("original hook not restored: content=%q err=%v", restored, err)
	}
	if err := UninstallMsgHook(ctx, dir, tr); err != nil {
		t.Fatal(err)
	}
	if restored, err := os.ReadFile(hookPath); err != nil || string(restored) != originalHook {
		t.Fatalf("repeated uninstall removed user hook: content=%q err=%v", restored, err)
	}
}

func TestInstallHook_PreservesExistingBackup(t *testing.T) {
	for _, name := range []string{"prepare-commit-msg", "commit-msg"} {
		t.Run(name, func(t *testing.T) {
			dir := initRepo(t)
			hookPath := filepath.Join(dir, ".git", "hooks", name)
			writeFile(t, dir, ".git/hooks/"+name, "custom active hook")
			writeFile(t, dir, ".git/hooks/"+name+".bak", "original backup")
			tr := i18n.New(i18n.LocaleEN)
			var err error
			if name == "commit-msg" {
				err = InstallMsgHook(context.Background(), dir, "", tr)
			} else {
				err = InstallHook(context.Background(), dir, false, "", tr)
			}
			if err == nil {
				t.Fatal("install must reject replacing an unrelated backup")
			}
			for path, want := range map[string]string{hookPath: "custom active hook", hookPath + ".bak": "original backup"} {
				if got, readErr := os.ReadFile(path); readErr != nil || string(got) != want {
					t.Fatalf("hook data changed: %s content=%q err=%v", path, got, readErr)
				}
			}
		})
	}
}
