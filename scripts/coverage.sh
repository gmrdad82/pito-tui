#!/usr/bin/env bash
# Run the full suite with the race detector and enforce the coverage floor
# CI requires. Same script locally and in ci.yml so the two can never
# disagree about what "enough" means.
set -euo pipefail
cd "$(dirname "$0")/.."

FLOOR=80

go test -race -coverprofile=cover.out ./...
total=$(go tool cover -func=cover.out | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
echo "total coverage: ${total}%"

if awk -v t="$total" -v f="$FLOOR" 'BEGIN { exit !(t+0 < f) }'; then
  echo "coverage ${total}% is below the ${FLOOR}% floor" >&2
  exit 1
fi
