---
name: mnemon
description: >
  Persistent memory system for LLM agents. Use this skill whenever you need to
  remember facts, decisions, or context across sessions, recall past knowledge,
  check for duplicate memories, or manage memory lifecycle. Activates on topics
  like "remember this", "what did we decide", "past context", "save this fact",
  or any situation where cross-session knowledge would help.
---

# Memory Skill — mnemon

You have access to a persistent memory system via the `mnemon` CLI.
You MUST actively use it to store and retrieve knowledge across sessions.

## On every conversation start (MANDATORY)

```bash
mnemon recall "<topic or project name>" --smart --limit 5
```

Load relevant context before responding.

## When to remember (MANDATORY — do not skip)

You MUST run `mnemon diff` + `mnemon remember` when ANY of these occur:

1. **User states a preference** — tool choice, coding style, workflow, naming convention
2. **An architectural or design decision is made** — why X over Y, trade-offs discussed
3. **A bug is diagnosed and fixed** — root cause, fix approach, lessons learned
4. **A new project pattern is established** — file structure, API convention, testing approach
5. **User corrects you** — the correction itself is a preference or fact worth saving
6. **Key facts are discovered** — API specs, version constraints, environment details
7. **A task is completed** — summarize what was built/changed for future context
8. **User expresses ongoing interest in a topic** — save as preference

### How to remember

```bash
# 1. ALWAYS check for duplicates first
mnemon diff "<new fact>"

# 2. Based on suggestion:
#    ADD      → mnemon remember "<fact>" --cat <category> --imp <1-5>
#    CONFLICT → mnemon forget <old_id> && mnemon remember "<updated>" --cat <cat> --imp <n>
#    DUPLICATE→ skip
```

### Entity extraction

When calling `remember`, extract key entities from the content and pass them via `--entities`:

```bash
mnemon remember "Chose Qdrant over Milvus for vector search" \
  --cat decision --imp 5 \
  --entities "Qdrant,Milvus,vector-search"
```

The `--entities` flag accepts comma-separated entities that get merged with auto-extracted entities.

## When the user asks about past context

```bash
mnemon recall "<query>" --smart --limit 10
```

## Categories

| Category | Use for | Default importance |
|----------|---------|-------------------|
| `preference` | User likes/dislikes, tool choices, workflow | 4 |
| `decision` | Architectural or design decisions with rationale | 5 |
| `fact` | Objective information, specs, environment details | 3 |
| `insight` | Patterns, lessons learned, debugging techniques | 4 |
| `context` | Project state, current phase, WIP status | 3 |
| `general` | Anything else | 3 |

## Importance scale

- `5` critical — core decisions, strong user preferences
- `4` high — important facts, recurring patterns
- `3` medium — general context (default)
- `2` low — minor details
- `1` trivial — ephemeral notes

## Linking (after remember)

When `mnemon remember` outputs candidates, evaluate them:

```bash
# Semantic links — for topically related insights
mnemon link <new_id> <candidate_id> --type semantic --weight 0.85

# Causal links — when one insight caused/enabled/prevented another
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'
```

Skip candidates with only superficial overlap.

## Retention management

```bash
mnemon gc --threshold 0.4          # list low-retention candidates
mnemon gc --keep <id>              # boost retention (+3 access)
mnemon forget <id>                 # soft-delete an insight
```

## Other commands

```bash
mnemon search "<query>" --limit 10    # token-scored search
mnemon related <id> --edge causal     # find related insights via graph
mnemon status                         # memory statistics
mnemon log                            # recent operations
mnemon embed --status                 # embedding coverage
mnemon embed --all                    # backfill embeddings (requires Ollama)
```

## Content size limits

- Single insight: max **8,000 characters**
- For longer content, chunk into multiple `remember` calls at semantic boundaries
- Each chunk should be self-contained and independently meaningful

## Rules

- ALWAYS `diff` before `remember` to avoid duplicates
- Use `--smart` on recall for intent-aware retrieval
- Prefer specific categories over `general`
- Set importance >= 4 for decisions and strong preferences
- Do NOT store secrets, passwords, or tokens
