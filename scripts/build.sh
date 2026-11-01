#!/usr/bin/env bash
# Builds the release binary:
#   npm ci → vite build (with typecheck) → embed check → go test → go build
#
# Environment:
#   VERSION, COMMIT   injected into the binary (default: from git)
#   OUT               output path (default: .tmp/bin/mihomo-visz)
#   GOOS, GOARCH      honoured by go build for cross-compiling
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
OUT="${OUT:-.tmp/bin/mihomo-visz}"

echo "==> frontend"
(
  cd frontend
  npm ci --cache ../.tmp/npm-cache
  npm run build
)

echo "==> embed check"
./scripts/check-embed.sh

echo "==> go test"
CGO_ENABLED=0 go test ./...

echo "==> go build ${VERSION} (${COMMIT})"
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
  -o "${OUT}" ./cmd/mihomo-visz

echo "==> ${OUT}"
