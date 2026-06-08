#!/usr/bin/env bash
set -euo pipefail

# Run from repository root (folder containing this script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Building and installing commitgen..."

if ! command -v go >/dev/null 2>&1; then
  echo "Error: Go is not installed or not in PATH." >&2
  exit 1
fi

# Install binary
GO111MODULE=on go install ./cmd/commitgen

# Resolve install bin directory
GOBIN_VALUE="$(go env GOBIN)"
if [ -n "$GOBIN_VALUE" ]; then
  BIN_DIR="$GOBIN_VALUE"
else
  GOPATH_VALUE="$(go env GOPATH)"

  # GOPATH can contain multiple paths.
  # Windows typically uses ';', Unix uses ':'.
  if [[ "$GOPATH_VALUE" == *';'* ]]; then
    FIRST_GOPATH="${GOPATH_VALUE%%;*}"
  else
    FIRST_GOPATH="${GOPATH_VALUE%%:*}"
  fi

  BIN_DIR="$FIRST_GOPATH/bin"
fi

GOOS_VALUE="$(go env GOOS)"
if [ "$GOOS_VALUE" = "windows" ]; then
  BIN_NAME="commitgen.exe"
else
  BIN_NAME="commitgen"
fi

TARGET="$BIN_DIR/$BIN_NAME"

echo "✅ Successfully installed to: $TARGET"
echo "You can run it with: commitgen"

echo
if ! command -v commitgen >/dev/null 2>&1; then
  echo "Note: 'commitgen' is not in your current PATH yet."
  echo "Add this directory to PATH: $BIN_DIR"
fi
