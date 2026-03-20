#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="${1:-go-api-server}"
TAG="${2:-latest}"
PLATFORM="${3:-linux/amd64}"
OUTPUT_TAR="${4:-${IMAGE_NAME}_${TAG}.tar}"

echo "========================================"
echo "Docker image build/export script"
echo "Image     : ${IMAGE_NAME}:${TAG}"
echo "Platform  : ${PLATFORM}"
echo "Output tar: ${OUTPUT_TAR}"
echo "========================================"

docker version >/dev/null

echo "[1/2] Build Linux image..."
docker buildx build --platform "${PLATFORM}" -t "${IMAGE_NAME}:${TAG}" --load .

echo "[2/2] Save image to tar..."
docker save -o "${OUTPUT_TAR}" "${IMAGE_NAME}:${TAG}"

echo
echo "Done."
echo "Created: ${OUTPUT_TAR}"
echo "Ubuntu import command:"
echo "  docker load -i ${OUTPUT_TAR}"
