#!/usr/bin/env bash

set -eu

IMAGE=docker.io/coosis/k8s-vllm-router:dev

podman build -t "${IMAGE}" -f deploy/docker/router.Dockerfile .
rm -f build/k8s-vllm-router.tar || true
podman save "${IMAGE}" -o build/k8s-vllm-router-dev.tar
limactl copy build/k8s-vllm-router-dev.tar k3s:/tmp/k8s-vllm-router-dev.tar
limactl shell k3s sudo k3s ctr images import /tmp/k8s-vllm-router-dev.tar
