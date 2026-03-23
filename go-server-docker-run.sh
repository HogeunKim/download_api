#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------
# Ubuntu script: load Docker image tar and run container
# Usage:
#   chmod +x go-server-docker-run.sh
#   ./go-server-docker-run.sh go-api-server_1.0.1.tar /home/genes007/api
#
# Required argument:
#   $1 : image tar file name/path (ex: go-api-server_1.0.1.tar)
#   $2 : host output dir path (ex: /home/genes007/api)
#
# Optional env overrides:
#   HOST_PORT=9800
#   CONTAINER_PORT=9800
#   USE_SUDO_DOCKER=false
# ---------------------------------------------

IMAGE_TAR_INPUT="${1:-}"
HOST_OUTPUT_DIR_INPUT="${2:-}"
HOST_PORT="${HOST_PORT:-9800}"
CONTAINER_PORT="${CONTAINER_PORT:-9800}"
USE_SUDO_DOCKER="${USE_SUDO_DOCKER:-false}"
CONTAINER_NAME="go-api-server"

if [[ -z "${IMAGE_TAR_INPUT}" ]]; then
  echo "Usage: ./go-server-docker-run.sh <IMAGE_TAR> <HOST_OUTPUT_DIR>" >&2
  echo "[error] IMAGE_TAR argument is required. Example: go-api-server_1.0.1.tar" >&2
  exit 1
fi
if [[ -z "${HOST_OUTPUT_DIR_INPUT}" ]]; then
  echo "Usage: ./go-server-docker-run.sh <IMAGE_TAR> <HOST_OUTPUT_DIR>" >&2
  echo "[error] HOST_OUTPUT_DIR argument is required. Example: /home/genes007/api" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

if [[ "${IMAGE_TAR_INPUT}" = /* ]]; then
  IMAGE_TAR="${IMAGE_TAR_INPUT}"
else
  IMAGE_TAR="${SCRIPT_DIR}/${IMAGE_TAR_INPUT}"
fi

if [[ ! -f "${IMAGE_TAR}" ]]; then
  echo "[error] Docker image tar not found: ${IMAGE_TAR}" >&2
  exit 1
fi

IMAGE_TAR_BASE="$(basename "${IMAGE_TAR}")"
IMAGE_TAR_STEM="${IMAGE_TAR_BASE%.tar}"
IMAGE_REPO="${IMAGE_TAR_STEM%_*}"
IMAGE_TAG="${IMAGE_TAR_STEM##*_}"
if [[ -z "${IMAGE_REPO}" || -z "${IMAGE_TAG}" || "${IMAGE_REPO}" == "${IMAGE_TAR_STEM}" ]]; then
  echo "[error] Invalid tar file name format: ${IMAGE_TAR_BASE}" >&2
  echo "        Expected format: <image-name>_<version>.tar (e.g. go-api-server_1.0.1.tar)" >&2
  exit 1
fi
IMAGE_NAME="${IMAGE_REPO}:${IMAGE_TAG}"

HOST_OUTPUT_DIR_CLEAN="${HOST_OUTPUT_DIR_INPUT%/}"
if [[ -z "${HOST_OUTPUT_DIR_CLEAN}" ]]; then
  echo "[error] HOST_OUTPUT_DIR is invalid: ${HOST_OUTPUT_DIR_INPUT}" >&2
  exit 1
fi
HOST_OUTPUT_BASENAME="$(basename "${HOST_OUTPUT_DIR_CLEAN}")"
if [[ -z "${HOST_OUTPUT_BASENAME}" || "${HOST_OUTPUT_BASENAME}" == "." || "${HOST_OUTPUT_BASENAME}" == "/" ]]; then
  echo "[error] failed to derive folder name from HOST_OUTPUT_DIR: ${HOST_OUTPUT_DIR_CLEAN}" >&2
  exit 1
fi
CONTAINER_OUTPUT_DIR="/data/${HOST_OUTPUT_BASENAME}"

if [[ "${USE_SUDO_DOCKER}" == "true" ]]; then
  DOCKER="sudo docker"
else
  DOCKER="docker"
fi

echo "========================================"
echo "[run] Start"
echo "[run] tar            : ${IMAGE_TAR}"
echo "[run] image          : ${IMAGE_NAME}"
echo "[run] container      : ${CONTAINER_NAME}"
echo "[run] port mapping   : ${HOST_PORT}:${CONTAINER_PORT}"
echo "[run] volume mapping : ${HOST_OUTPUT_DIR_CLEAN} -> ${CONTAINER_OUTPUT_DIR}"
echo "========================================"

echo "[1/5] Ensure output directory"
mkdir -p "${HOST_OUTPUT_DIR_CLEAN}"

echo "[2/5] Docker load"
${DOCKER} load -i "${IMAGE_TAR}"

echo "[3/5] Stop old container (if running): ${CONTAINER_NAME}"
${DOCKER} stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true

echo "[4/5] Remove old container (if exists): ${CONTAINER_NAME}"
${DOCKER} rm "${CONTAINER_NAME}" >/dev/null 2>&1 || true

echo "[5/5] Run new container"
${DOCKER} run -d \
  --name "${CONTAINER_NAME}" \
  --restart unless-stopped \
  -p "${HOST_PORT}:${CONTAINER_PORT}" \
  -v "${HOST_OUTPUT_DIR_CLEAN}:${CONTAINER_OUTPUT_DIR}" \
  "${IMAGE_NAME}"

echo ""
echo "[run] Completed"
echo "[run] Verify:"
echo "  ${DOCKER} ps --filter name=${CONTAINER_NAME}"
echo "  ${DOCKER} logs --tail 200 ${CONTAINER_NAME}"
echo "  curl -s http://localhost:${HOST_PORT}/version"
