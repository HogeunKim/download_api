#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------
# Ubuntu script: load callback Docker image tar and run container on user-defined network
# Usage:
#   chmod +x run-docker-net-callback-server.sh
#   ./run-docker-net-callback-server.sh callback-server_0.0.2.tar
#
# Required argument:
#   $1 : image tar file name/path (ex: callback-server_0.0.2.tar)
#
# Optional env overrides:
#   NETWORK_NAME=app-net
#   HOST_PORT=9000
#   CONTAINER_PORT=9000
#   USE_SUDO_DOCKER=false
# ---------------------------------------------

IMAGE_TAR_INPUT="${1:-}"
NETWORK_NAME="${NETWORK_NAME:-app-net}"
HOST_PORT="${HOST_PORT:-9000}"
CONTAINER_PORT="${CONTAINER_PORT:-9000}"
USE_SUDO_DOCKER="${USE_SUDO_DOCKER:-false}"
CONTAINER_NAME="callback-server"

if [[ -z "${IMAGE_TAR_INPUT}" ]]; then
  echo "Usage: ./run-docker-net-callback-server.sh <IMAGE_TAR>" >&2
  echo "[error] IMAGE_TAR argument is required. Example: callback-server_0.0.2.tar" >&2
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
  echo "        Expected format: <image-name>_<version>.tar (e.g. callback-server_0.0.2.tar)" >&2
  exit 1
fi
IMAGE_NAME="${IMAGE_REPO}:${IMAGE_TAG}"

if [[ "${USE_SUDO_DOCKER}" == "true" ]]; then
  DOCKER="sudo docker"
else
  DOCKER="docker"
fi

echo "========================================"
echo "[run] Start"
echo "[run] tar          : ${IMAGE_TAR}"
echo "[run] image        : ${IMAGE_NAME}"
echo "[run] container    : ${CONTAINER_NAME}"
echo "[run] network      : ${NETWORK_NAME}"
echo "[run] port mapping : ${HOST_PORT}:${CONTAINER_PORT}"
echo "========================================"

echo "[1/5] Ensure docker network exists"
${DOCKER} network inspect "${NETWORK_NAME}" >/dev/null 2>&1 || ${DOCKER} network create "${NETWORK_NAME}" >/dev/null

echo "[2/5] Docker load"
${DOCKER} load -i "${IMAGE_TAR}"

echo "[3/5] Stop old container (if running): ${CONTAINER_NAME}"
${DOCKER} stop "${CONTAINER_NAME}" >/dev/null 2>&1 || true

echo "[4/5] Remove old container (if exists): ${CONTAINER_NAME}"
${DOCKER} rm "${CONTAINER_NAME}" >/dev/null 2>&1 || true

echo "[5/5] Run new container"
${DOCKER} run -d \
  --name "${CONTAINER_NAME}" \
  --network "${NETWORK_NAME}" \
  --restart unless-stopped \
  -p "${HOST_PORT}:${CONTAINER_PORT}" \
  "${IMAGE_NAME}"

echo ""
echo "[run] Completed"
echo "[run] Verify:"
echo "  ${DOCKER} ps --filter name=${CONTAINER_NAME}"
echo "  ${DOCKER} inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${CONTAINER_NAME}"
echo "  ${DOCKER} logs --tail 200 ${CONTAINER_NAME}"
echo "  curl -s http://localhost:${HOST_PORT}/events"
