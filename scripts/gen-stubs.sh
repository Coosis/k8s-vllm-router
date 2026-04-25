#!/bin/bash
# Generate type stubs for pyright

cd "$(dirname "$0")/.."

echo "Generating type stubs..."

pyright --createstub httpx

echo "Stubs generated in ./stubs/"
echo ""
echo "To regenerate (e.g., after package updates):"
echo "  rm -rf stubs/*"
echo "  ./scripts/generate-stubs.sh"
