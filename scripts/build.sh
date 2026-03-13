#!/usr/bin/env bash
# Build script for MasterDnsVPN Resolver Scanner CLI.
# Cleans asset files, then cross-compiles all release targets.
#
# Usage (from repo root):
#   ./scripts/build.sh
#
# Output: cli/bin/scanner-*

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_DIR="$SCRIPT_DIR/../cli"
BIN_DIR="$CLI_DIR/bin"

cd "$CLI_DIR"

echo "[info] cleaning asset files..."
go run ./tools/cleanassets
echo ""

echo "[info] building release targets..."
mkdir -p "$BIN_DIR"

build() {
    local goos="$1" goarch="$2" out="$3"
    printf "  %-40s" "$out"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -o "$BIN_DIR/$out" .
    echo "[ok]"
}

build linux   amd64 scanner-linux-amd64
build linux   arm64 scanner-linux-arm64
build windows amd64 scanner-windows-amd64.exe
build darwin  amd64 scanner-macos-amd64
build darwin  arm64 scanner-macos-arm64
build android arm64 scanner-android-arm64

echo ""
echo "[ok] all targets built in cli/bin/"
ls -lh "$BIN_DIR"
