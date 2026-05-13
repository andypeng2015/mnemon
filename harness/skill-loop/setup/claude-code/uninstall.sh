#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Remove the Claude Code Mnemon skill loop integration.

Usage:
  uninstall.sh [--global] [--config-dir DIR] [--purge-library]

By default, uninstall removes hooks, protocol skills, subagent, and generated
host skill views but preserves mnemon-skill-loop/skills.
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR=".claude"
PURGE_LIBRARY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --global)
      CONFIG_DIR="${HOME}/.claude"
      shift
      ;;
    --config-dir)
      CONFIG_DIR="${2:?missing value for --config-dir}"
      shift 2
      ;;
    --purge-library)
      PURGE_LIBRARY=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required to update Claude Code settings.json" >&2
  exit 1
fi

ENV_PATH="${CONFIG_DIR}/mnemon-skill-loop/env.sh"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi
HOST_SKILLS_DIR="${MNEMON_SKILL_LOOP_HOST_SKILLS_DIR:-${CONFIG_DIR}/skills}"

python3 "${SCRIPT_DIR}/scripts/update_settings.py" uninstall --config-dir "${CONFIG_DIR}"

if [[ -d "${HOST_SKILLS_DIR}" ]]; then
  while IFS= read -r marker; do
    rm -rf "$(dirname "${marker}")"
  done < <(find "${HOST_SKILLS_DIR}" -mindepth 2 -maxdepth 2 -name .mnemon-skill-loop-generated -print 2>/dev/null)
fi

rm -rf "${CONFIG_DIR}/hooks/mnemon-skill-loop"
rm -rf "${HOST_SKILLS_DIR}/skill_observe"
rm -rf "${HOST_SKILLS_DIR}/skill_curate"
rm -rf "${HOST_SKILLS_DIR}/skill_manage"
rm -f "${CONFIG_DIR}/agents/mnemon-skill-curator.md"

if [[ "${PURGE_LIBRARY}" == "1" ]]; then
  rm -rf "${CONFIG_DIR}/mnemon-skill-loop"
else
  rm -f "${CONFIG_DIR}/mnemon-skill-loop/GUIDE.md"
  rm -f "${CONFIG_DIR}/mnemon-skill-loop/env.local.sh"
  rmdir "${CONFIG_DIR}/mnemon-skill-loop/reports" 2>/dev/null || true
  rmdir "${CONFIG_DIR}/mnemon-skill-loop/proposals" 2>/dev/null || true
  rmdir "${CONFIG_DIR}/mnemon-skill-loop" 2>/dev/null || true
fi

echo "Removed Mnemon skill loop from ${CONFIG_DIR}."
