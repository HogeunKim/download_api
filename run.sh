#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

TARGET="${1:-server}"

case "$TARGET" in
  server)
    BIN="./bin/go-api-server-linux-amd64"
    ;;
  callback)
    BIN="./bin/callback-server-linux-amd64"
    ;;
  *)
    echo "Usage: ./run.sh [server|callback]"
    exit 1
    ;;
esac

if [[ ! -f "$BIN" ]]; then
  echo "Binary not found: $BIN"
  echo "먼저 빌드하세요. (예: ./build-all.ps1로 linux 바이너리 생성)"
  exit 1
fi

if [[ ! -x "$BIN" ]]; then
  chmod +x "$BIN"
fi

exec "$BIN"
