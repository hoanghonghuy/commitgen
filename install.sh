#!/bin/bash
set -e

echo "Building and installing commitgen..."
go install ./cmd/commitgen

TARGET=$(go env GOPATH)/bin/commitgen
echo "✅ Successfully installed to: $TARGET"
echo "You can run it with: commitgen"
