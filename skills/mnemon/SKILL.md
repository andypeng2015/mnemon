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
Use it to store and retrieve knowledge across sessions.

## On every conversation start

```bash
mnemon recall "<topic or project name>" --smart --limit 5
```

Load relevant context before responding.

## What to remember

Memory bridges the gap between sessions. Store information that is **costly to
re-obtain** — either the user would have to repeat themselves, or you would have
to redo significant work.

Three types of information are worth persisting:

### 1. User directives

Things the user explicitly expressed — you cannot derive these on your own.

- **Preferences**: tool choices, coding style, communication style, risk tolerance
- **Decisions**: choices the user made with their rationale ("use SQLite because single-binary")
- **Corrections**: when the user corrects your behavior or output
- **Constraints**: boundaries, requirements, goals ("never auto-commit", "budget under $10K")

→ `--cat preference` or `--cat decision`, `--imp 4-5`

### 2. Reasoning conclusions

Insights you derived through analysis — re-deriving them costs significant effort.

- **Root cause analysis**: "503 errors caused by connection pool exhaustion under concurrent load"
- **Design rationale**: "chose event sourcing over CRUD because of audit trail requirements"
- **Comparative analysis**: "Qdrant outperforms Milvus on filtered search by 3x in our benchmark"
- **Pattern discovery**: "sector rotation signal preceded 5% corrections 3 times in Q1"

→ `--cat insight` or `--cat decision`, `--imp 4-5`

### 3. Observed state

Facts about the environment that you discovered — not easily re-observable
or not recorded elsewhere.

- **System topology**: "Service A depends on B and C; B uses a Redis cluster"
- **Environment specifics**: "production DB is PostgreSQL 15 on RDS us-east-1, read replicas in us-west-2"
- **Domain context**: "portfolio is 60/30/10 equity/bonds/alternatives"
- **Historical state**: "last P1 incident was Feb 3, caused by DNS resolution timeout"

→ `--cat fact` or `--cat context`, `--imp 3`

### What NOT to remember

- **Easily re-derivable** — reading code, checking docs, or a quick search can recover it
- **Transient state** — current task progress, intermediate results that will change
- **Public knowledge** — common facts, well-documented APIs, widely-known patterns
- **Every small task** — git log already tracks what changed; don't duplicate it

**The test**: if you forget it, does the user have to repeat themselves or do you
have to redo significant work? If no, don't store it.

## How to remember

```bash
# 1. ALWAYS check for duplicates first
mnemon diff "<new fact>"

# 2. Based on suggestion:
#    ADD      → mnemon remember "<fact>" --cat <category> --imp <1-5>
#    CONFLICT → mnemon forget <old_id> && mnemon remember "<updated>" --cat <cat> --imp <n>
#    DUPLICATE→ skip
```

### Entity extraction

Extract key entities from the content and pass them via `--entities`:

```bash
mnemon remember "Chose Qdrant over Milvus for vector search" \
  --cat decision --imp 5 \
  --entities "Qdrant,Milvus,vector-search"
```

## When the user asks about past context

```bash
mnemon recall "<query>" --smart --limit 10
```

## Categories

| Category | Maps to | Typical importance |
|----------|---------|-------------------|
| `preference` | User directives (preferences, corrections) | 4 |
| `decision` | User directives or reasoning conclusions (decisions with rationale) | 5 |
| `insight` | Reasoning conclusions (analysis, root causes, patterns) | 4 |
| `fact` | Observed state (environment facts, domain context) | 3 |
| `context` | Observed state (system state, historical events) | 3 |

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
- Do NOT store secrets, passwords, or tokens
