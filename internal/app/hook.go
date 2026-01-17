package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// InstallHook installs the prepare-commit-msg hook
func InstallHook(ctx context.Context) error {
	// 1. Detect .git directory
	gitDir := ".git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("current directory is not a git repository root (no .git found)")
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "prepare-commit-msg")

	// 2. Check if hook exists
	if _, err := os.Stat(hookPath); err == nil {
		// Hook exists. We should not overwrite blindly.
		// For now, let's error out or ask user (but this is a command).
		// Let's notify user.
		return fmt.Errorf("hook %s already exists. Please remove it first", hookPath)
	}

	// 3. Create hook script
	// We need the absolute path to commitgen binary?
	// Or assume it's in PATH.
	// Since we are running the binary, we can try `os.Executable()`.
	exe, err := os.Executable()
	if err != nil {
		exe = "commitgen" // fallback
	} else {
		// Evaluate symlinks if needed, but absolute path is safer.
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

echo "🤖 commitgen is analyzing changes..."
"%s" --hook "$COMMIT_MSG_FILE" < /dev/tty > /dev/tty

# If commitgen succeeds, it writes to the file.
`, exe)

	if err := os.WriteFile(hookPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write hook file: %w", err)
	}

	fmt.Printf("✅ Hook installed to %s\n", hookPath)
	return nil
}
