#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------
# Ubuntu script: load Docker image tar and run container
# Usage:
#   chmod +x docker-load-run.sh
#   ./docker-load-run.sh <HOST_OUTPUT_DIR>
#   ./docker-load-run.sh /home/genes007/download_result
#
# Optional env overrides:
#   IMAGE_TAR=go-api-server_latest.tar
#   IMAGE_NAME=go-api-server:latest
#   CONTAINER_NAME=go-api-server
#   HOST_PORT=9800
#   CONTAINER_PORT=9800
#   USE_SUDO_DOCKER=false
# ---------------------------------------------

IMAGE_TAR="${IMAGE_TAR:-go-api-server_latest.tar}"
IMAGE_NAME="${IMAGE_NAME:-go-api-server:latest}"
CONTAINER_NAME="${CONTAINER_NAME:-go-api-server}"
HOST_PORT="${HOST_PORT:-9800}"
CONTAINER_PORT="${CONTAINER_PORT:-9800}"
HOST_OUTPUT_DIR_INPUT="${1:-}"
USE_SUDO_DOCKER="${USE_SUDO_DOCKER:-false}"

if [[ -z "${HOST_OUTPUT_DIR_INPUT}" ]]; then
  echo "Usage: ./docker-load-run.sh <HOST_OUTPUT_DIR>" >&2
  echo "[error] HOST_OUTPUT_DIR argument is required." >&2
  exit 1
fi

HOST_OUTPUT_DIR="${HOST_OUTPUT_DIR_INPUT%/}"
if [[ -z "${HOST_OUTPUT_DIR}" ]]; then
  echo "[error] HOST_OUTPUT_DIR is invalid: ${HOST_OUTPUT_DIR_INPUT}" >&2
  exit 1
fi

HOST_OUTPUT_BASENAME="$(basename "${HOST_OUTPUT_DIR}")"
if [[ -z "${HOST_OUTPUT_BASENAME}" || "${HOST_OUTPUT_BASENAME}" == "." || "${HOST_OUTPUT_BASENAME}" == "/" ]]; then
  echo "[error] failed to derive folder name from HOST_OUTPUT_DIR: ${HOST_OUTPUT_DIR}" >&2
  exit 1
fi
CONTAINER_OUTPUT_DIR="/data/${HOST_OUTPUT_BASENAME}"

if [[ "${USE_SUDO_DOCKER}" == "true" ]]; then
  DOCKER="sudo docker"
else
  DOCKER="docker"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

if [[ ! -f "${IMAGE_TAR}" ]]; then
  echo "[error] Docker image tar not found: ${IMAGE_TAR}" >&2
  exit 1
fi

echo "========================================"
echo "[load-run] Start"
echo "[load-run] tar            : ${IMAGE_TAR}"
echo "[load-run] image          : ${IMAGE_NAME}"
echo "[load-run] container      : ${CONTAINER_NAME}"
echo "[load-run] port mapping   : ${HOST_PORT}:${CONTAINER_PORT}"
echo "[load-run] volume mapping : ${HOST_OUTPUT_DIR} -> ${CONTAINER_OUTPUT_DIR}"
echo "========================================"

echo "[1/4] Ensure output directory"
mkdir -p "${HOST_OUTPUT_DIR}"

echo "[2/4] Docker load"
${DOCKER} load -i "${IMAGE_TAR}"

echo "[3/4] Remove old container (if exists)"
${DOCKER} rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true

echo "[4/4] Run new container"
${DOCKER} run -d \
  --name "${CONTAINER_NAME}" \
  --restart unless-stopped \
  -p "${HOST_PORT}:${CONTAINER_PORT}" \
  -v "${HOST_OUTPUT_DIR}:${CONTAINER_OUTPUT_DIR}" \
  "${IMAGE_NAME}"

echo ""
echo "[load-run] Completed"
echo "[load-run] Verify:"
echo "  ${DOCKER} ps --filter name=${CONTAINER_NAME}"
echo "  ${DOCKER} logs --tail 200 ${CONTAINER_NAME}"
echo "  curl -s http://localhost:${HOST_PORT}/version"
