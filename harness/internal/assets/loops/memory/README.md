# Mnemon Memory Loop Harness

This directory is the canonical memory loop template. It is host-agnostic: a
capable host agent can read these Markdown assets, while host adapters project
the loop into concrete runtimes such as Claude Code or Codex.

## File Tree

```text
harness/internal/assets/loops/memory/
├── README.md
├── loop.json
├── env.sh
├── GUIDE.md
├── MEMORY.md
├── hooks/
│   └── intents.json
├── skills/
│   ├── memory-get/
│   │   └── SKILL.md
│   └── memory-set/
│       └── SKILL.md
```

## Core Parts

| Part | Role |
| --- | --- |
| HostAgent | The host agent runtime. It owns task execution, model judgment, and native hook/skill/subagent mechanisms. |
| `MEMORY.md` | Prompt-facing mirror generated from scoped Local Mnemon memory. |
| Local Mnemon | Local memory source. It accepts local candidates and serves scoped reads without a Remote Workspace. |

## Support Assets

| Asset | Purpose |
| --- | --- |
| `loop.json` | Machine-readable loop manifest for standard lifecycle events, assets, state, and host adapters. |
| `env.sh` | Runtime config: memory directory, env path, and mirror size threshold. |
| `GUIDE.md` | Policy: when to read memory, when to write memory, and what is worth keeping. |
| `hooks/intents.json` | Declarative hook intents; the generated hook shells for Prime, Remind, Nudge, and Compact render from these plus host mechanics. |
| `skills/memory-get/SKILL.md` | Scoped memory read skill backed by `mnemon-harness control pull`. |
| `skills/memory-set/SKILL.md` | Local memory candidate write skill backed by `mnemon-harness control observe`. |
| Host adapter | Host-specific projection lives outside the loop under `harness/internal/assets/hosts/<host>/`. |

## Runtime Directory Protocol

All reusable assets resolve their runtime files through one environment
config file and environment variables:

```text
$MNEMON_MEMORY_LOOP_DIR/
├── env.sh
├── GUIDE.md
└── MEMORY.md
```

`env.sh` defines:

```bash
MNEMON_MEMORY_LOOP_ENV=<project>/.mnemon/harness/memory/env.sh
MNEMON_MEMORY_LOOP_DIR=<project>/.mnemon/harness/memory
MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES=200
```

`memory-set`, `memory-get`, and hooks should never hard-code a host path. They
should source `.mnemon/harness/local/env.sh` when it is available and use
`$MNEMON_MEMORY_LOOP_DIR` only as the mirror/guide location. If the host runtime
cannot pass environment variables to skills, the Prime hook must inject the
resolved path into the HostAgent context.

`MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES` controls when hook prompts should note
that the mirror is becoming large.

## Boundary

The harness does not provide a custom agent runtime. It provides Markdown
materials that a HostAgent can mount into its existing instruction, hook, skill,
and subagent systems.

The key split is:

```text
GUIDE.md decides when memory behavior is useful.
memory-get maps read-memory behavior to Local Mnemon pull.
memory-set maps write-memory behavior to Local Mnemon observe.
MEMORY.md is a generated mirror, not a write target.
```

## Claude Code Install

Install into the current project:

```bash
go run ./harness/cmd/mnemon-harness setup --host claude-code --memory --project-root .
```

Remove the installed Claude Code integration while preserving `MEMORY.md`:

```bash
go run ./harness/cmd/mnemon-harness setup uninstall --host claude-code --memory --principal claude-code@project --project-root .
```
