#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Install the Mnemon skill loop harness into Claude Code.

Usage:
  install.sh [--global] [--config-dir DIR] [--host-skills-dir DIR]
             [--with-remind] [--no-nudge] [--no-compact]

Defaults:
  --config-dir .claude
  --host-skills-dir <config-dir>/skills
  installs Prime, Nudge, and Compact hooks; Remind is disabled by default

Examples:
  bash harness/skill-loop/setup/claude-code/install.sh
  bash harness/skill-loop/setup/claude-code/install.sh --global
  bash harness/skill-loop/setup/claude-code/install.sh --host-skills-dir .claude/skills
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HARNESS_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CONFIG_DIR=".claude"
HOST_SKILLS_DIR=""
ENABLE_REMIND=0
ENABLE_NUDGE=1
ENABLE_COMPACT=1

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
    --host-skills-dir)
      HOST_SKILLS_DIR="${2:?missing value for --host-skills-dir}"
      shift 2
      ;;
    --with-remind)
      ENABLE_REMIND=1
      shift
      ;;
    --no-nudge)
      ENABLE_NUDGE=0
      shift
      ;;
    --no-compact)
      ENABLE_COMPACT=0
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

if [[ -z "${HOST_SKILLS_DIR}" ]]; then
  HOST_SKILLS_DIR="${CONFIG_DIR}/skills"
fi

mkdir -p \
  "${CONFIG_DIR}/mnemon-skill-loop/skills/active" \
  "${CONFIG_DIR}/mnemon-skill-loop/skills/stale" \
  "${CONFIG_DIR}/mnemon-skill-loop/skills/archived" \
  "${CONFIG_DIR}/mnemon-skill-loop/proposals" \
  "${CONFIG_DIR}/mnemon-skill-loop/reports" \
  "${HOST_SKILLS_DIR}/skill_observe" \
  "${HOST_SKILLS_DIR}/skill_curate" \
  "${HOST_SKILLS_DIR}/skill_manage" \
  "${CONFIG_DIR}/agents" \
  "${CONFIG_DIR}/hooks/mnemon-skill-loop"

install_file() {
  local src="$1"
  local dst="$2"
  local mode="$3"
  cp "$src" "$dst"
  chmod "$mode" "$dst"
}

install_file "${HARNESS_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-skill-loop/GUIDE.md" 0644
if [[ ! -f "${CONFIG_DIR}/mnemon-skill-loop/env.sh" ]]; then
  install_file "${HARNESS_DIR}/env.sh" "${CONFIG_DIR}/mnemon-skill-loop/env.sh" 0755
fi

DEFAULT_HOST_SKILLS_DIR="${CONFIG_DIR}/skills"
if [[ "${HOST_SKILLS_DIR}" != "${DEFAULT_HOST_SKILLS_DIR}" ]]; then
  cat > "${CONFIG_DIR}/mnemon-skill-loop/env.local.sh" <<EOF
export MNEMON_SKILL_LOOP_HOST_SKILLS_DIR="${HOST_SKILLS_DIR}"
EOF
fi

install_file "${HARNESS_DIR}/skills/skill_observe.md" "${HOST_SKILLS_DIR}/skill_observe/SKILL.md" 0644
install_file "${HARNESS_DIR}/skills/skill_curate.md" "${HOST_SKILLS_DIR}/skill_curate/SKILL.md" 0644
install_file "${HARNESS_DIR}/skills/skill_manage.md" "${HOST_SKILLS_DIR}/skill_manage/SKILL.md" 0644
install_file "${HARNESS_DIR}/subagents/curator.md" "${CONFIG_DIR}/agents/mnemon-skill-curator.md" 0644

install_file "${SCRIPT_DIR}/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-skill-loop/prime.sh" 0755
install_file "${SCRIPT_DIR}/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-skill-loop/remind.sh" 0755
install_file "${SCRIPT_DIR}/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-skill-loop/nudge.sh" 0755
install_file "${SCRIPT_DIR}/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-skill-loop/compact.sh" 0755

python3 "${SCRIPT_DIR}/scripts/update_settings.py" install \
  --config-dir "${CONFIG_DIR}" \
  --remind "${ENABLE_REMIND}" \
  --nudge "${ENABLE_NUDGE}" \
  --compact "${ENABLE_COMPACT}"

HOOK_SUMMARY="prime"
if [[ "${ENABLE_REMIND}" == "1" ]]; then
  HOOK_SUMMARY="${HOOK_SUMMARY}, remind"
fi
if [[ "${ENABLE_NUDGE}" == "1" ]]; then
  HOOK_SUMMARY="${HOOK_SUMMARY}, nudge"
fi
if [[ "${ENABLE_COMPACT}" == "1" ]]; then
  HOOK_SUMMARY="${HOOK_SUMMARY}, compact"
fi

cat <<EOF
Installed Mnemon skill loop for Claude Code.

Config:       ${CONFIG_DIR}
Guide:        ${CONFIG_DIR}/mnemon-skill-loop/GUIDE.md
Env:          ${CONFIG_DIR}/mnemon-skill-loop/env.sh
Library:      ${CONFIG_DIR}/mnemon-skill-loop/skills/{active,stale,archived}
Usage:        ${CONFIG_DIR}/mnemon-skill-loop/skills/.usage.jsonl
Proposals:    ${CONFIG_DIR}/mnemon-skill-loop/proposals
Host skills:  ${HOST_SKILLS_DIR}
Protocols:    ${HOST_SKILLS_DIR}/skill_observe/SKILL.md
              ${HOST_SKILLS_DIR}/skill_curate/SKILL.md
              ${HOST_SKILLS_DIR}/skill_manage/SKILL.md
Agent:        ${CONFIG_DIR}/agents/mnemon-skill-curator.md
Hooks:        ${HOOK_SUMMARY}

Restart Claude Code to load new skills and subagents.
EOF
