#!/bin/bash
PROMPT_DIR="${HOME}/.mnemon/prompt"

echo "[mnemon] Memory active"
[ -f "${PROMPT_DIR}/guide.md" ] && cat "${PROMPT_DIR}/guide.md"
