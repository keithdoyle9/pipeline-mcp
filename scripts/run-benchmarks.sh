#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Unit tests run in the verify workflow and verify-release.sh. This script owns
# only the reproducible benchmark corpus.
go run ./cmd/benchmark
