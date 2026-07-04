#!/usr/bin/env bash
# TUI capture rig — the terminal twin of pito's `rake pito:capture`
# (ferrum drives a browser there; vhs drives a headless terminal here).
# Scenarios are .tape files in captures/; artifacts land in
# tmp/captures/<name>/. Usage:
#   scripts/capture.sh list
#   scripts/capture.sh <name>
set -euo pipefail
cd "$(dirname "$0")/.."

case "${1:-list}" in
  list)
    for tape in captures/*.tape; do
      name=$(basename "$tape" .tape)
      steps=$(grep -c -v -e '^#' -e '^$' "$tape")
      printf '%-12s (%s lines)\n' "$name" "$steps"
    done
    ;;
  *)
    name="$1"
    tape="captures/$name.tape"
    [ -f "$tape" ] || { echo "no scenario $name (try: scripts/capture.sh list)" >&2; exit 1; }
    go build -o pito-tui ./cmd/pito-tui
    out="tmp/captures/$name"
    mkdir -p "$out"
    (cd "$out" && vhs "../../../$tape")
    echo "artifacts:"
    ls -1 "$out"
    ;;
esac
