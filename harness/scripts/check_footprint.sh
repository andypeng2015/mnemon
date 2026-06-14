#!/usr/bin/env bash
set -euo pipefail
base="${1:-origin/master}"
out="$(git diff --name-only "$base" -- ':!harness/' ':!go.mod' ':!go.sum' ':!docs/harness' ':!docs/zh/harness')"
[ -z "$out" ] && { echo "footprint clean vs $base"; exit 0; }
echo "FOOTPRINT VIOLATION vs $base:"; echo "$out"; exit 1
