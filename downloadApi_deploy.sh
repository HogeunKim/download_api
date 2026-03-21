#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------
# Ubuntu deployment script for downloadApi
# Usage:
#   chmod +x downloadApi_deploy.sh
#   ./downloadApi_deploy.sh
#
# Optional env overrides:
#   BRANCH=main
#   IMAGE_NAME=go-api-server:latest
#   CONTAINER_NAME=go-api-server
#   HOST_PORT=9800
#   CONTAINER_PORT=9800
#   HOST_OUTPUT_DIR=/home/genes007/download_api/download_result
#   CONTAINER_OUTPUT_DIR=/home/genes007/download_api/download_api
#   USE_SUDO_DOCKER=true
# ---------------------------------------------

BRANCH="${BRANCH:-main}"
IMAGE_NAME="${IMAGE_NAME:-go-api-server:latest}"
CONTAINER_NAME="${CONTAINER_NAME:-go-api-server}"
HOST_PORT="${HOST_PORT:-9800}"
CONTAINER_PORT="${CONTAINER_PORT:-9800}"
HOST_OUTPUT_DIR="${HOST_OUTPUT_DIR:-/home/genes007/download_api/download_result}"
CONTAINER_OUTPUT_DIR="${CONTAINER_OUTPUT_DIR:-/home/genes007/download_api/download_api}"
USE_SUDO_DOCKER="${USE_SUDO_DOCKER:-true}"

if [[ "${USE_SUDO_DOCKER}" == "true" ]]; then
  DOCKER="sudo docker"
else
  DOCKER="docker"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "========================================"
echo "[deploy] downloadApi deployment start"
echo "[deploy] repo dir      : ${SCRIPT_DIR}"
echo "[deploy] branch        : ${BRANCH}"
echo "[deploy] image         : ${IMAGE_NAME}"
echo "[deploy] container     : ${CONTAINER_NAME}"
echo "[deploy] port mapping  : ${HOST_PORT}:${CONTAINER_PORT}"
echo "[deploy] volume mapping: ${HOST_OUTPUT_DIR} -> ${CONTAINER_OUTPUT_DIR}"
echo "========================================"

echo "[1/6] Git fetch"
git fetch origin

echo "[2/6] Git checkout ${BRANCH}"
git checkout "${BRANCH}"

echo "[3/6] Git pull origin ${BRANCH}"
git pull origin "${BRANCH}"

echo "[4/6] Prepare output directory"
mkdir -p "${HOST_OUTPUT_DIR}"
chmod 777 "${HOST_OUTPUT_DIR}"

echo "[5/6] Docker build"
${DOCKER} build -t "${IMAGE_NAME}" .

echo "[6/6] Recreate container"
${DOCKER} rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
${DOCKER} run -d \
  --name "${CONTAINER_NAME}" \
  --restart unless-stopped \
  -p "${HOST_PORT}:${CONTAINER_PORT}" \
  -v "${HOST_OUTPUT_DIR}:${CONTAINER_OUTPUT_DIR}" \
  "${IMAGE_NAME}"

echo ""
echo "[deploy] Completed"
echo "[deploy] Verify commands:"
echo "  ${DOCKER} ps"
echo "  ${DOCKER} logs --tail 200 ${CONTAINER_NAME}"
