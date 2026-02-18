#!/bin/bash
# mnemon Stop hook: evaluate whether to persist memory
# This script is kept as a fallback. The recommended approach is
# the prompt-type hook configured in settings.json, which uses
# an LLM to evaluate conversation content semantically.

INPUT=$(cat)
STOP_HOOK_ACTIVE=$(echo "$INPUT" | jq -r '.stop_hook_active // false' 2>/dev/null)

if [ "$STOP_HOOK_ACTIVE" = "true" ]; then
  exit 0
fi

# Fallback: just pass through. Semantic evaluation is handled by prompt hook.
exit 0
