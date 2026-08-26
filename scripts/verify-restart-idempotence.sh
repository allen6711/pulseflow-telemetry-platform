#!/usr/bin/env bash
# Runs five consecutive up/down cycles and records whether each produced an
# identical result (SC-010, FR-004).
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

CYCLES="${CYCLES:-5}"
OUT_DIR="${OUT_DIR:-benchmarks/raw}"
mkdir -p "$OUT_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="$OUT_DIR/restart-idempotence-$STAMP.txt"

SERVICES=(kafka clickhouse redis prometheus api worker)
failures=0

{
  echo "measurement: repeated start/stop consistency (SC-010)"
  echo "timestamp_utc: $STAMP"
  echo "cycles: $CYCLES"
  echo "commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo
} | tee "$OUT_FILE"

for cycle in $(seq 1 "$CYCLES"); do
  echo "==> Cycle $cycle/$CYCLES" | tee -a "$OUT_FILE"

  docker compose down --remove-orphans --volumes >/dev/null 2>&1 || true

  start=$(date +%s)
  if docker compose up -d --build --wait >/dev/null 2>&1; then
    result=ok
  else
    result=FAILED
    failures=$((failures + 1))
  fi
  elapsed=$(($(date +%s) - start))

  unhealthy=""
  for svc in "${SERVICES[@]}"; do
    state="$(docker compose ps --format '{{.Service}} {{.Health}} {{.State}}' 2>/dev/null \
      | awk -v s="$svc" '$1==s {print ($2 != "" ? $2 : $3)}')"
    case "$state" in
      healthy|running) ;;
      *) unhealthy="$unhealthy $svc=$state" ;;
    esac
  done

  if [ -n "$unhealthy" ]; then
    result=FAILED
    failures=$((failures + 1))
  fi

  echo "    result=$result elapsed=${elapsed}s unhealthy=${unhealthy:-none}" | tee -a "$OUT_FILE"
done

{
  echo
  echo "cycles_run: $CYCLES"
  echo "cycles_failed: $failures"
} | tee -a "$OUT_FILE"

echo
echo "==> Raw output written to $OUT_FILE"
[ "$failures" -eq 0 ]
