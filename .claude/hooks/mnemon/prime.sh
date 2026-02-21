#!/bin/bash
PROMPT_DIR="${HOME}/.mnemon/prompt"

STATS=$(mnemon status 2>/dev/null)
if [ -n "$STATS" ]; then
  INSIGHTS=$(echo "$STATS" | grep -o '"total_insights": *[0-9]*' | grep -o '[0-9]*')
  EDGES=$(echo "$STATS" | grep -o '"edge_count": *[0-9]*' | grep -o '[0-9]*')
  echo "[mnemon] Memory active (${INSIGHTS} insights, ${EDGES} edges)."
else
  echo "[mnemon] Memory active."
fi

[ -f "${PROMPT_DIR}/guide.md" ] && cat "${PROMPT_DIR}/guide.md"
