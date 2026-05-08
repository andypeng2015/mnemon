# 05. Working Memory、Consolidation、Long-Term Memory 与 Eval

## Core Model

Mnemon memory uses cognitive names for architecture and engineering names for implementation:

```text
Cognitive model:
Working Memory  <->  Memory Consolidation  <->  Long-Term Memory

Engineering model:
Prompt Memory   <->  Dreaming Jobs         <->  Mnemon Store + Skills
```

The older hot/cold wording is only a storage analogy. The canonical design is:

| Cognitive role | Engineering implementation | Filesystem owner | Purpose |
|---|---|---|---|
| Working Memory | Prompt Memory / Markdown Memory | `memory/prompt/` | small, high-confidence memory injected into the host prompt |
| Episodic Memory | Evidence / Event Log | `memory/longterm/episodic/` | events, transcripts, tool outputs, decisions, failures |
| Semantic Memory | Mnemon Store | `memory/longterm/semantic/` | facts, preferences, summaries, project knowledge, indexes |
| Procedural Memory | Skills | `skills/` | reusable workflows, tactics, procedures, habits |
| Memory Consolidation | Dreaming Jobs | `memory/consolidation/`, `reports/dreaming/` | compact, archive, extract, promote, and propose skills |

This keeps the mental model clear without forcing brain-science terms into every schema and path.

## Working Memory / Prompt Memory

Working Memory is the bounded Markdown memory directly loaded into the host agent's prompt. It follows the practical pattern used by Claude-style agents and Hermes: a small set of durable facts and preferences, not a database.

Hermes baseline:

| Mechanism | Hermes behavior |
|---|---|
| Files | `MEMORY.md`, `USER.md` |
| Location | `~/.hermes/memories/` |
| Budget | about 2,200 chars for `MEMORY.md`, 1,375 chars for `USER.md` |
| Loading | frozen snapshot injected into system prompt at session start |
| Updates | `add`, `replace`, `remove` through a memory tool |
| Overflow | reject write, ask the agent to consolidate/replace first |
| Format | entries separated by `§` |
| Safety | prompt-injection/secret/invisible-char scanning before accept |

Mnemon Prompt Memory keeps this shape:

```text
memory/prompt/
  MEMORY.md
  USER.md
  project.md
```

Prompt Memory properties:

- Markdown.
- Small and explicitly budgeted.
- Fully loaded into the host prompt or project instruction snapshot.
- Directly model-facing.
- Highest reliability recall path.
- Agent-curated through explicit memory tools or hooks.
- Current user request always wins.
- Not a transcript, diary, evidence store, or task log.

Prompt Memory should contain:

- stable user preferences;
- durable project facts;
- environment facts the agent repeatedly needs;
- short high-confidence constraints;
- compact lessons that are not better represented as skills.

Prompt Memory should not contain:

- raw transcripts;
- long logs;
- one-off task progress;
- temporary TODOs;
- low-confidence inference;
- procedural workflows that should become skills.

## Long-Term Memory

Long-Term Memory is not one storage mechanism. It is a role split across Mnemon Store and Skills:

```text
Long-Term Memory
  episodic  -> Mnemon evidence/event storage
  semantic  -> Mnemon facts/summaries/preferences/indexes
  procedural -> skills
```

Mnemon Store owns episodic and semantic memory:

```text
memory/longterm/
  episodic/
    evidence/
    transcripts/
    events/
    decisions/
    failures/
  semantic/
    facts/
    preferences/
    summaries/
    topics/
    index/
  archive/
    prompt/
  imports/
```

Skills own procedural memory:

```text
skills/
  core/
  project/
  generated/
    active/
    quarantine/
    candidates/
  archive/
```

Long-Term Memory properties:

- Large capacity.
- Long retention.
- Searchable and rankable.
- Not fully loaded into prompt.
- Can store raw evidence and long histories.
- Can use Mnemon, RAG, SQLite/FTS, vector search, graph storage, or another backend.
- Lower immediate reliability than Prompt Memory because recall is selective.
- Source of candidates for Prompt Memory promotion and skill creation.

Long-Term Memory is not "bad memory". Prompt Memory is small and high-performance; Long-Term Memory is larger, longer-lived, and retrieved only when relevant.

## Daily Write Path

Foreground agents should not perform semantic long-term writes by default. Daily memory writes are deliberately simple:

```text
interaction
  -> append low-cost evidence/event log
  -> maintain Prompt Memory when explicitly asked or when the host memory tool permits it
  -> defer semantic extraction and skill generation to Dreaming Jobs
```

The evidence log is required even when semantic writes are deferred. Without source evidence, later consolidation becomes unsupported summary.

Evidence event shape:

```yaml
type: evidence_event
timestamp: 2026-05-09T00:00:00Z
source: post_tool_call|user_correction|turn_summary|failure|manual_import
scope:
  user: optional
  project: optional
  branch: optional
summary: "The build failed because pnpm was missing from PATH."
refs:
  transcript: memory/longterm/episodic/transcripts/session-abc.md
  tool_call: optional
sensitivity: public|internal|secret-redacted
candidate_for:
  - semantic
  - skill
```

This gives Dreaming Jobs durable raw material without forcing the active agent to decide every semantic write in real time.

## Memory Consolidation / Dreaming Jobs

Memory Consolidation is implemented as Dreaming Jobs. Dreaming is not a free-form background agent; it is a set of scoped jobs with schemas, budgets, reports, and write allowlists.

Dreaming job types:

| Job | Reads | Writes | Purpose |
|---|---|---|---|
| `compact` | `memory/prompt/**` | prompt patch proposal | keep Working Memory under quota |
| `archive` | prompt entries, evidence events | `memory/longterm/archive/prompt/**` | preserve demoted prompt memory |
| `extract` | evidence, transcripts, summaries | semantic memory proposal | turn evidence into facts/preferences/summaries |
| `promote` | semantic memory, recall hits, user confirmations | prompt patch proposal | reactivate durable facts into Working Memory |
| `skill-candidate` | repeated workflows, failures, tool traces | `skills/generated/candidates/**` | turn procedures into reviewable skills |

Triggers:

- Prompt Memory quota pressure.
- Task end or session end.
- Failure review.
- Important user correction.
- Repeated recall hit.
- Scheduled/idle runner tick.
- Manual curate/dream command.

Movement protocol:

| Gate | Direction | Trigger | Writes | Decision |
|---|---|---|---|---|
| G1 Capture | interaction -> episodic | observe/reflect/pre-compact/import | evidence events, transcripts, summaries | source/provenance recorded |
| G2 Compact | prompt -> prompt proposal | quota pressure/staleness/conflict | compact patch proposal | apply or report |
| G3 Extract | episodic -> semantic | dreaming detects stable fact | semantic proposal | store, reject, or ask review |
| G4 Promote | semantic -> prompt | high confidence/frequency/scope match | prompt patch proposal | apply or report |
| G5 Proceduralize | repeated experience -> skill | repeated workflow or tool tactic | skill candidate | review, activate, or archive |

The consolidation buffer lives under:

```text
memory/consolidation/
  candidates/
  summaries/
  promotions/
  demotions/
  decisions/
```

These are temporary or auditable staging artifacts. They do not define another memory tier.

## Prompt Admission Policy

Promotion to Prompt Memory requires stronger evidence than context recall.

Promotion triggers:

- user explicitly says to remember;
- same correction repeats across tasks;
- fact is reused frequently;
- semantic memory is high-confidence and current;
- Dreaming finds a stable pattern;
- recall keeps selecting the same long-term item and it proves useful.

Promotion gate:

```text
importance >= threshold
AND confidence >= threshold
AND recurrence >= threshold OR user_confirmed
AND risk <= allowed_risk
AND prompt_budget_available OR replacement_plan_exists
AND not better_as_skill
AND evidence_links_present
```

Promotion proposal:

```yaml
type: prompt_promotion
from:
  longterm_refs:
    - memory/longterm/semantic/summaries/session-2026-05-09.md
    - memory/longterm/episodic/evidence/build-failure-001.md
candidate: memory/consolidation/candidates/build-tooling.yaml
to: memory/prompt/project.md
reason: "Used in repeated build tasks and confirmed by user."
scores:
  importance: 0.86
  confidence: 0.91
  recurrence: 0.74
  recency: 0.83
  risk: 0.12
patch:
  action: add_or_replace
  content: "This repo uses pnpm for frontend package management."
```

## Prompt Eviction Policy

Prompt Memory is valuable because it stays small. It must have explicit eviction.

Demotion triggers:

- Prompt Memory exceeds budget;
- entry is stale or superseded;
- entry is too detailed;
- entry is rarely used;
- entry conflicts with newer user/project evidence;
- entry is procedural and should become a skill;
- entry is useful historically but not always needed in prompt.

Demotion gate:

```text
prompt_pressure >= threshold
OR stale == true
OR superseded == true
OR low_use_count == true
OR better_as_skill == true
```

Demotion proposal:

```yaml
type: prompt_demotion
from: memory/prompt/project.md
to:
  longterm_ref: memory/longterm/archive/prompt/project-2026-05-09.md
reason: "Too detailed for always-on prompt memory."
preserve:
  original_entry: true
  evidence_links: true
replacement:
  prompt_pointer: "Build details archived in long-term memory; recall when working on frontend tooling."
```

Default behavior is archive over delete.

## Recall From Long-Term Memory

Long-Term recall is retrieval, not memory loading.

Recall sources:

1. Prompt Memory is already in the prompt snapshot. It is checked for relevance, not retrieved.
2. Mnemon Store is the retrieval target for episodic and semantic memory.
3. Skills are discovered through the host skill system or skill index, not recalled as raw memory.
4. Consolidation artifacts are excluded from live recall by default.
5. `NONE` means no relevant prompt context and no long-term result above threshold.

Candidate ranking fields:

| Field | Meaning |
|---|---|
| `relevance` | lexical/semantic match to current task |
| `recency` | how recently the item was created/used/confirmed |
| `frequency` | how often it was useful |
| `confidence` | source quality and user confirmation |
| `scope_match` | user/project/repo/branch/session fit |
| `importance` | expected value if surfaced |
| `risk` | cost of injecting stale/wrong content |
| `budget_cost` | summary size |

Recall decision:

```text
score = relevance + recency + frequency + confidence + scope_match + importance
penalty = risk + budget_cost
return summary only if score - penalty >= threshold
otherwise return NONE
```

Recall output:

```yaml
type: longterm_recall
status: ok|none
summary: "..."
evidence:
  - memory/longterm/episodic/evidence/...
scores:
  relevance: 0.82
  confidence: 0.76
  risk: 0.18
promotion_candidate: true
```

Rules:

- raw transcript is never injected;
- recall is summarized and evidence-linked;
- current user request outranks recall;
- irrelevant long-term memory returns `NONE`;
- repeated useful recall can create a consolidation candidate;
- recall context is not automatically promoted to Prompt Memory.

## Skill Boundary

Promotion does not always mean Prompt Memory.

```text
fact / preference / compact constraint -> Prompt Memory
event / transcript / raw evidence -> Episodic Memory in Mnemon Store
summary / project knowledge / durable fact -> Semantic Memory in Mnemon Store
workflow / procedure / tool tactic -> Skill
uncertain inference -> report only
```

If evidence shows a repeated workflow, Dreaming should create a skill candidate, not a Prompt Memory entry.

## Curator Modes

Curator is a maintenance skill/hook. It can be triggered manually, by host scheduler, by external cron, or by the optional maintenance runner. It is not an agent loop and must not mutate active conversations.

Modes:

| Mode | Behavior |
|---|---|
| dry-run | read artifacts, write report |
| proposal | write structured proposals |
| apply | apply allowlisted low-risk patches after backup |
| rollback | restore from snapshot |

Inputs:

- `memory/prompt/**`
- long-term recall/index summaries
- `memory/consolidation/**`
- `state/usage.json`
- `state/pins.json`
- reports

Outputs:

- `reports/curator/<timestamp>.md`
- consolidation proposals
- optional Prompt Memory patches
- optional long-term archive writes
- updated sidecar

Curator rules:

- Prompt Memory budget is strict;
- default dry-run;
- archive over delete;
- back up before apply;
- skip pinned/user/imported unless approved;
- high-risk guideline/hook/install changes are proposal-only.

## Eval Gate

Eval-driven self-evolution is for higher-risk changes:

| Target | Risk | Gate |
|---|---|---|
| Prompt Memory entry | low/medium | budget + evidence + conflict check |
| long-term recall ranking | medium | regression recall cases |
| skill wording | low/medium | schema + sample task eval |
| hook prompt | medium | dry-run + regression cases |
| guideline | high | human approval |
| install map | high | install dry-run tests |
| code/scripts | high | tests + review |

Eval artifacts:

```text
eval/
  constraints.yaml
  datasets/
  results/
  templates/
    pr.md
```

Constraints example:

```yaml
constraints:
  max_prompt_memory_chars:
    MEMORY.md: 2200
    USER.md: 1375
    project.md: 4000
  max_prompt_growth: 0.2
  required_checks:
    - prompt-memory-budget
    - longterm-recall-regression
    - validate-skill
    - check-target-allowlist
    - report-schema
  protected_targets:
    - GUIDELINE.md
    - INSTALL.md
```

## Reports

Reports are the audit surface.

Every memory consolidation action must answer:

1. What changed or would change?
2. Was it prompt promotion, prompt demotion, long-term recall, semantic extraction, evidence capture, or skill proposal?
3. Why?
4. Which evidence supports it?
5. What scores and thresholds were used?
6. Was it applied or only proposed?
7. How can it be rolled back?

Report-first behavior is what keeps self-evolution reviewable.
