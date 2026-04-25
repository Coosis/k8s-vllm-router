#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${ROOT_DIR}/bin"
BIN="${BIN_DIR}/mock-backend"
PORTS="${MOCK_BACKEND_PORTS:-8091 8092 8093}"
PIDS=()
PID_TO_PORT=()

cleanup() {
  local status=$?
  trap - EXIT INT TERM

  if ((${#PIDS[@]} > 0)); then
    echo "stopping mock backends: ${PIDS[*]}"
    kill -TERM "${PIDS[@]}" 2>/dev/null || true
    sleep 0.2
    kill -KILL "${PIDS[@]}" 2>/dev/null || true
    wait "${PIDS[@]}" 2>/dev/null || true
  fi

  exit "${status}"
}

trap cleanup EXIT INT TERM

mkdir -p "${BIN_DIR}"
echo "building ${BIN}"
go build -o "${BIN}" ./cmd/mock-backend

for port in ${PORTS}; do
  MOCK_BACKEND_ADDR=":${port}" HOSTNAME="mock-${port}" "${BIN}" &
  pid=$!
  PIDS+=("${pid}")
  PID_TO_PORT+=("${pid}:${port}")
  echo "started mock backend on :${port} pid=${pid}"
done

echo
echo "router BACKENDS value:"
printf "BACKENDS="
sep=""
for port in ${PORTS}; do
  printf "%smock-%s=http://localhost:%s" "${sep}" "${port}" "${port}"
  sep=","
done
printf "\n\n"

echo "waiting; if any mock backend exits, all will be stopped"
while true; do
  running=" $(jobs -pr) "
  for pid in "${PIDS[@]}"; do
    if [[ "${running}" != *" ${pid} "* ]]; then
      failed_code=0
      wait "${pid}" || failed_code=$?
      failed_port="${pid}"
      for pair in "${PID_TO_PORT[@]}"; do
        if [[ "${pair}" == "${pid}:"* ]]; then
          failed_port="${pair#*:}"
          break
        fi
      done
      echo "mock backend on :${failed_port} pid=${pid} exited with status ${failed_code}"
      exit "${failed_code}"
    fi
  done
  sleep 0.2
done
