#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

IMAGE_NAME="${1:-go-api-server}"
TAG="${2:-latest}"
PLATFORM="${3:-linux/amd64}"
OUTPUT_TAR="${4:-${IMAGE_NAME}_${TAG}.tar}"
APP_VERSION="${5:-${TAG}}"

echo "========================================"
echo "Docker image build/export script"
echo "Image     : ${IMAGE_NAME}:${TAG}"
echo "AppVersion: ${APP_VERSION}"
echo "Platform  : ${PLATFORM}"
echo "Output tar: ${OUTPUT_TAR}"
echo "========================================"

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker command not found. Install Docker and verify PATH." >&2
  exit 1
fi

if ! docker version >/dev/null 2>&1; then
  echo "ERROR: Docker daemon is not reachable. Start Docker service/daemon first." >&2
  exit 1
fi

echo "[1/2] Build Linux image..."
docker buildx build --platform "${PLATFORM}" --build-arg "APP_VERSION=${APP_VERSION}" -t "${IMAGE_NAME}:${TAG}" --load .

echo "[2/2] Save image to tar..."
docker save -o "${OUTPUT_TAR}" "${IMAGE_NAME}:${TAG}"

if [[ ! -f "${OUTPUT_TAR}" ]]; then
  echo "ERROR: tar file was not created: ${OUTPUT_TAR}" >&2
  exit 1
fi

echo
echo "Done."
echo "Created: ${OUTPUT_TAR}"
echo "Ubuntu import command:"
echo "  docker load -i ${OUTPUT_TAR}"
