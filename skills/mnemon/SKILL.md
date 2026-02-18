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

You are a memory-aware agent. You recall past context at the start of
every conversation, and actively evaluate whether each exchange produces
knowledge worth persisting across sessions.

Use the `mnemon` CLI to manage your persistent memory.

## Protocol

### Step 1 — Conversation start: Load context

```bash
mnemon recall "<topic or project name>" --smart --limit 5
```

### Step 2 — Before finishing each response: Evaluate

After completing your response to each user message, ask yourself:

> Did this exchange produce anything worth remembering next session?

**Remember if ANY of these occurred:**

- **User directive** — the user stated a preference, decision, correction, or constraint
- **Reasoning conclusion** — you completed a non-trivial analysis, comparison, diagnosis, or design evaluation
- **Observed state** — you discovered a system fact, environment detail, or domain-specific state not recorded elsewhere

**Skip if ALL of these are true:**

- Easily re-derivable by reading code, docs, or a quick search
- Transient — current task progress, intermediate results, temporary state
- Public knowledge, common patterns, or already tracked by git history

**The test**: if you forget it, does the user have to repeat themselves or do you have to redo significant work?

### Step 3 — Remember

```bash
# 1. Check for duplicates
mnemon diff "<new fact>"

# 2. Act on the suggestion:
#    ADD      → mnemon remember "<fact>" --cat <category> --imp <1-5> --entities "e1,e2"
#    CONFLICT → mnemon forget <old_id> && mnemon remember "<updated>" --cat <cat> --imp <n>
#    DUPLICATE→ skip
```

### Step 4 — Link

When `mnemon remember` outputs link candidates, evaluate and connect meaningful relationships:

```bash
# Semantic — topically related insights
mnemon link <new_id> <candidate_id> --type semantic --weight 0.85

# Causal — one insight caused/enabled/prevented another
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'
```

Skip candidates with only superficial overlap.

## Categories

| Category | What it captures | Typical importance |
|----------|-----------------|:------------------:|
| `preference` | User preferences, corrections, style choices | 4 |
| `decision` | Decisions with rationale (user's or yours) | 5 |
| `insight` | Analysis results, root causes, comparisons, patterns | 4 |
| `fact` | Environment facts, system topology, domain context | 3 |
| `context` | Historical state, events, situational details | 3 |

## Commands reference

```bash
mnemon recall "<query>" --smart --limit 10   # intent-aware retrieval
mnemon search "<query>" --limit 10           # keyword search
mnemon related <id> --edge causal            # graph traversal
mnemon gc --threshold 0.4                    # low-retention candidates
mnemon gc --keep <id>                        # boost retention
mnemon forget <id>                           # soft-delete
mnemon status                                # memory stats
mnemon log                                   # recent operations
mnemon embed --all                           # backfill embeddings (requires Ollama)
```

## Rules

- ALWAYS `diff` before `remember` — no duplicates
- ALWAYS use `--smart` on recall
- Prefer specific categories over `general`
- Do NOT store secrets, passwords, or tokens
- Max 8,000 chars per insight — chunk longer content at semantic boundaries
