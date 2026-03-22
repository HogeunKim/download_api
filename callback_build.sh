#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

BUILD_DIR="$SCRIPT_DIR/build"
BIN_PATH="$BUILD_DIR/callback-server"

mkdir -p "$BUILD_DIR"

echo "[callback] build 시작..."
go build -o "$BIN_PATH" ./cmd/callback-server
chmod +x "$BIN_PATH"
echo "[callback] build 완료: $BIN_PATH"
