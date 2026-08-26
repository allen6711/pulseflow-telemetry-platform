#!/usr/bin/env bash
# Measures time from a cold stack to every service reporting healthy (SC-001).
#
# Constitution Principle III: this script exists so the measurement is
# reproducible, and it records the conditions alongside the number. The result
# is data, not a target -- nothing here asserts a pass or fail threshold.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

OUT_DIR="${OUT_DIR:-benchmarks/raw}"
mkdir -p "$OUT_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="$OUT_DIR/cold-start-$STAMP.txt"

echo "==> Tearing the stack down and clearing build cache for a cold start"
docker compose down --remove-orphans --volumes >/dev/null 2>&1 || true

echo "==> Starting; waiting for every service to report healthy"
START=$(date +%s)
docker compose up -d --build --wait
END=$(date +%s)
ELAPSED=$((END - START))

{
  echo "measurement: cold start to all-healthy (SC-001)"
  echo "timestamp_utc: $STAMP"
  echo "elapsed_seconds: $ELAPSED"
  echo
  echo "command: docker compose up -d --build --wait"
  echo
  echo "host:"
  echo "  uname: $(uname -srm)"
  echo "  cpus: $(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown)"
  echo "  docker: $(docker --version)"
  echo "  compose: $(docker compose version --short 2>/dev/null || echo unknown)"
  echo "  go: $(go version 2>/dev/null || echo 'not installed')"
  echo
  echo "git:"
  echo "  commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo "  dirty: $([ -n "$(git status --porcelain 2>/dev/null)" ] && echo yes || echo no)"
  echo
  echo "service health:"
  docker compose ps --format 'table {{.Service}}\t{{.State}}\t{{.Status}}'
} | tee "$OUT_FILE"

echo
echo "==> Elapsed ${ELAPSED}s. Raw output written to $OUT_FILE"
