#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="$ROOT_DIR/.gocache" go test ./... -run Test -count=1

GOCACHE="$ROOT_DIR/.gocache" go run ./cmd/benchmark
