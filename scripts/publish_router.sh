#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Build and publish the router image to Docker Hub.

Usage:
  scripts/publish_router.sh

Environment:
  CONTAINER_TOOL  Container CLI to use. Defaults to podman, then docker.
  REGISTRY        Registry host. Defaults to docker.io.
  REPOSITORY      Image repository. Defaults to coosis/k8s-vllm-router.
  TAG             Image tag. Defaults to latest.
  IMAGE           Full image reference. Overrides REGISTRY/REPOSITORY/TAG.
  PLATFORM        Optional build platform, for example linux/amd64.

Examples:
  TAG=v0.1.0 scripts/publish_router.sh
  CONTAINER_TOOL=docker TAG="$(git rev-parse --short HEAD)" scripts/publish_router.sh
  PLATFORM=linux/amd64 TAG=v0.1.0 scripts/publish_router.sh
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "${CONTAINER_TOOL:-}" ]]; then
  if command -v podman >/dev/null 2>&1; then
    CONTAINER_TOOL=podman
  elif command -v docker >/dev/null 2>&1; then
    CONTAINER_TOOL=docker
  else
    echo "error: neither podman nor docker is available" >&2
    exit 1
  fi
fi

REGISTRY="${REGISTRY:-docker.io}"
REPOSITORY="${REPOSITORY:-coosis/k8s-vllm-router}"
TAG="${TAG:-latest}"
IMAGE="${IMAGE:-${REGISTRY}/${REPOSITORY}:${TAG}}"

build_args=(-t "${IMAGE}" -f deploy/docker/router.Dockerfile)
if [[ -n "${PLATFORM:-}" ]]; then
  build_args+=(--platform "${PLATFORM}")
fi

echo "Building ${IMAGE}"
"${CONTAINER_TOOL}" build "${build_args[@]}" .

echo "Publishing ${IMAGE}"
"${CONTAINER_TOOL}" push "${IMAGE}"

echo "Published ${IMAGE}"
