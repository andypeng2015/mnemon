#!/bin/bash
# mnemon SessionStart hook: auto-recall past context
# Runs on every session start, injects relevant memories as context.

INPUT=$(cat)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty' 2>/dev/null)
PROJECT=$(basename "${CWD:-$PWD}")

RESULT=$(mnemon recall "$PROJECT" --smart --limit 5 2>/dev/null)

if [ -n "$RESULT" ] && ! echo "$RESULT" | grep -qi "no insights found"; then
  ESCAPED=$(echo "$RESULT" | jq -Rs .)
  echo "{\"additionalContext\": $ESCAPED}"
fi
