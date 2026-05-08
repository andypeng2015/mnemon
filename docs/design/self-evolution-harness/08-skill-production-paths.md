# 08. Skill Self-Evolution Architecture

The harness treats skills as procedural memory. Memory stores stable facts, preferences, and compact context. Skills store reusable procedures, operational strategies, tool workflows, failure recovery paths, and task-class tactics.

The Hermes lesson is not "build a larger skill runtime." The lesson is:

```text
experience signal
  -> classify memory vs skill vs session note
  -> patch an existing class-level skill first
  -> create a new skill only when a reusable class of work exists
  -> record provenance and usage outside SKILL.md
  -> let curator consolidate self-authored sediment later
```

## Core Boundary

```text
facts / preferences / stable project context -> memory
procedures / workflows / repeated tactics -> skill
raw evidence / transcript / failed attempts -> episodic long-term memory
task continuity -> session summary
skill overlap / stale self-authored behavior -> curator
```

Skill production must be conservative. A system that creates one skill per turn becomes noisy and harder to use. The default is:

1. patch an existing skill;
2. add a support file under an existing umbrella skill;
3. create a new class-level skill only when no existing skill covers the behavior;
4. write a proposal report when evidence is weak or write restrictions are unavailable;
5. let curator archive or consolidate self-authored skills later.

## Production And Governance Model

Hermes effectively has three skill production entrances and one governance path:

| Layer | Trigger | Producer | Output | Provenance | Auto-curation |
|---|---|---|---|---|---|
| User-declared production | user explicitly asks to save/update a procedure | foreground host agent | protected skill patch/create or proposal | `user` / `foreground` | no by default |
| Agent-offered production | foreground agent asks after a difficult or iterative task, then user confirms | foreground host agent | protected skill patch/create or proposal | `agent` + `foreground_confirmed` | manual-review by default |
| Background review production | `turn_delivered`, `Stop`, `SessionEnd`, or queued reflection | restricted review agent or reflect job | self-authored patch, candidate skill, support file, or report | `agent` + `reflection` | yes, if not pinned/protected |
| Curator governance | idle/scheduled/manual maintenance | curator or dreaming job | umbrella consolidation, archive, demotion, promotion, or report | `agent` + `curator` / `dreaming` | yes, within allowlist |

The first three paths create or patch skill artifacts from recent experience. Curator is different: it governs skill sediment across time. It can still produce a new umbrella skill, but its primary job is library health, not direct per-turn learning.

## Artifact Model

The harness should keep the Hermes artifact shape but move the source of truth into `.mnemon`:

```text
.mnemon/
  skills/
    core/
      install/SKILL.md
      recall/SKILL.md
      observe/SKILL.md
      reflect/SKILL.md
      curate/SKILL.md
    project/
      <user-or-project-skill>/SKILL.md
    generated/
      candidates/
      quarantine/
      active/
    archive/
  state/
    usage.json
    lineage.json
    pins.json
  reports/
    reflection/
    curator/
```

Each skill is a directory:

```text
<skill>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

`SKILL.md` is model-facing procedural guidance. Sidecar state is engineering-facing governance metadata. The two should not be mixed.

Recommended limits follow the Hermes/Claude-style progressive disclosure model:

| Field | Policy |
|---|---|
| `name` | lowercase slug, stable, class-level, max 64 chars |
| `description` | discovery summary, max 1024 chars |
| `SKILL.md` | concise trigger, workflow, pitfalls, verification; large detail moves to support files |
| support files | `references/`, `templates/`, `scripts/`, `assets/`; bounded size and schema checked |
| model-facing metadata | YAML frontmatter only for discovery and compatibility |
| governance metadata | `state/usage.json`, `state/lineage.json`, `state/pins.json` |

## Skill Index And Write Surface

The harness needs two logical APIs, even when implemented as Markdown instructions or CLI commands rather than native tools:

```text
skill_index:
  list -> name, description, category, state
  view -> SKILL.md
  view_file -> support file by relative path

skill_manage:
  create
  patch
  edit
  write_file
  remove_file
  archive
```

Rules:

- list returns metadata only;
- view loads full `SKILL.md`;
- support files load on demand;
- patch is preferred over edit;
- archive is preferred over delete;
- delete should not exist as an automatic operation;
- every write records provenance and report evidence;
- foreground/user-created skills are protected by default;
- self-authored reflection skills are curator-eligible by default.

## Path A: User-Declared Production

User-declared production happens when the user explicitly asks the agent to save or update a procedure.

Examples:

- "把这个流程写成 skill";
- "记住以后这个项目要这样发布";
- "更新 debug skill，加上这个坑";
- "把刚才的安装步骤整理成一个可复用技能。"

Pipeline:

```text
explicit user request
  -> identify target skill or new class
  -> read existing skill index
  -> patch existing skill when possible
  -> create project skill only if needed
  -> write report
  -> mark protected/manual-review
```

Rules:

- user intent wins over curator preference;
- foreground user-created skills belong to the user;
- automatic curator must not rewrite or archive them without approval;
- package/core/harness skills may be patched only through explicit approved upgrade flow;
- any hook, install, permission, or guideline change requires human approval.

Foreground provenance:

```yaml
created_by: user
provenance: foreground
curation_policy: protected
review_required: false
```

## Path B: Agent-Offered Production

Agent-offered production happens during foreground work when the agent notices reusable procedural value and asks the user before saving.

Hermes does this through the `skill_manage` tool description: after difficult or iterative tasks, offer to save; skip simple one-offs; confirm with the user before creating or deleting.

Trigger signals:

- complex task succeeded after several tool calls;
- a non-trivial error path was overcome;
- the user corrected the workflow and the corrected approach worked;
- a recurring project workflow became clear;
- a loaded skill was missing an important step.

Pipeline:

```text
foreground work
  -> detect reusable workflow
  -> ask user whether to save/update a skill
  -> if confirmed, search skill index
  -> patch existing skill first
  -> create new skill only for a reusable class
  -> mark protected/manual-review
```

Rules:

- no confirmation means no durable skill write;
- the saved skill should describe a task class, not the exact session;
- the body should include trigger conditions, steps, pitfalls, and verification;
- session-specific detail should move to `references/`;
- this path is not silently auto-curated because it is foreground/user-confirmed.

Foreground-confirmed provenance:

```yaml
created_by: agent
provenance: foreground_confirmed
confirmed_by_user: true
curation_policy: manual-review
```

## Path C: Background Review Production

Background review is the Hermes-style self-improvement loop. It runs after the active task completes, so it can inspect outcomes without competing with the user's current request.

Host implementations differ:

| Host capability | Implementation |
|---|---|
| Background review agent | fork a restricted review agent after stop |
| Hook-capable host | run `reflect` hook with write allowlist |
| Weak host | enqueue `reflect.deferred` job for runner/manual processing |

Reflection input:

- bounded turn summary or transcript window;
- tool outcomes and failures;
- user corrections;
- skills loaded or viewed during the turn;
- current skill index metadata;
- write allowlist and protected-target list.

Pipeline:

```text
turn delivered
  -> run restricted reflect prompt
  -> classify insight
  -> memory / skill / session note / evidence / report-only
  -> inspect loaded skill first
  -> inspect existing umbrella skill next
  -> patch or write support file
  -> create candidate only if no umbrella fits
  -> validate schema and target
  -> write sidecar + report
```

Review constraints:

- it cannot talk to the user;
- it cannot continue the user task;
- it cannot call arbitrary tools;
- it cannot patch protected targets;
- it must prefer currently-loaded skills;
- it must prefer existing umbrella skills;
- it must write a report for every proposal or mutation;
- if write-target restrictions are unavailable, it must be proposal-only.

Background provenance:

```yaml
created_by: agent
provenance: reflection
curation_policy: auto-curatable
state: candidate|quarantined|active
```

## Path D: Curator Governance

Curator is not a fourth per-turn production path. It is the library governance path.

Inputs:

- `state/usage.json`;
- `state/lineage.json`;
- `state/pins.json`;
- active and candidate skills;
- reflection reports;
- curator reports;
- memory consolidation candidates;
- long-term evidence index.

Outputs:

- umbrella skill proposal;
- duplicated skill consolidation;
- stale skill archive proposal;
- support-file demotion;
- candidate promotion;
- quarantine or archive decision;
- curator report.

Pipeline:

```text
idle / scheduled / manual curator
  -> apply deterministic usage transitions
  -> scan self-authored skills only
  -> skip pinned/user/package/imported
  -> cluster overlap by task class
  -> patch umbrella or create umbrella
  -> archive absorbed skills
  -> write structured report
```

Curator rules:

- default dry-run;
- snapshot before apply;
- archive over delete;
- skip pinned skills;
- skip user-created, package/core, imported, and protected skills;
- consolidate by human-maintainer shape, not exact name similarity;
- prefer support files for narrow but valuable session-specific detail;
- every absorbed skill records `absorbed_into`;
- every archive has a restore path.

## Creation Gates

Every path should pass the same gates:

| Gate | Requirement |
|---|---|
| Reuse | repeated pattern, explicit user request, or strong project-level workflow |
| Scope | clear trigger and bounded responsibility |
| Evidence | links to report, session summary, or evidence event |
| Non-overlap | existing skill index checked first |
| Shape | class-level name, concise body, support files for detail |
| Size | under configured limits |
| Safety | no secrets, no unreviewed policy or permission change |
| Provenance | `created_by`, `provenance`, `state`, `created_at`, evidence refs recorded |

## Patch Policy

Patch before create.

Patch candidates:

- add one discovered caveat;
- update command preference;
- add a failure recovery path;
- clarify when the skill should not be used;
- broaden a trigger for a real task class;
- add a pointer to a support file;
- move detailed examples into `references/`.

Avoid patching when:

- the evidence is single-use and weak;
- the patch would turn the skill into a transcript;
- the patch conflicts with user-authored instructions;
- the target skill is package-provided and not forked;
- the target is protected and the user did not approve.

Pinned skills should be protected from archive/delete. Patching pinned skills may still be allowed when the owner explicitly requested the improvement.

## Provenance And Curation

Recommended provenance values:

| `created_by` | `provenance` | Meaning | Automated mutation |
|---|---|---|---|
| `harness` | `package` | shipped by harness package | no |
| `user` | `foreground` | explicitly authored by user | no |
| `agent` | `foreground_confirmed` | foreground agent saved after user confirmation | manual-review |
| `agent` | `reflection` | post-turn self-authored | yes, if not pinned/protected |
| `agent` | `curator` | maintenance-authored umbrella or patch | yes, if not pinned/protected |
| `agent` | `dreaming` | synthesized from accumulated evidence | proposal first |
| `external` | `imported` | imported from another package/repo | no |

Auto-curation eligibility:

```text
created_by == "agent"
AND provenance in {"reflection", "curator", "dreaming"}
AND pinned != true
AND state in {"candidate", "quarantined", "active", "stale"}
AND target not protected
```

## Lifecycle

Agent-authored skills should not immediately become first-class durable behavior unless the host/user explicitly requested that. Reflection and dreaming outputs start as candidates or quarantined skills:

```yaml
state: candidate|quarantined|active|stale|archived
lineage:
  created_from:
    - reports/reflection/2026-05-08.md
    - memory/longterm/episodic/evidence/...
  replaces: []
  absorbed_from: []
  absorbed_into: null
  promoted_by: null
```

Recommended lifecycle:

```text
candidate proposal
  -> quarantine if auto-written
  -> active after human approval, repeated use, or eval pass
  -> stale when usage drops or superseded
  -> archived after curator report + backup
```

Quarantine rules:

- quarantined skills are discoverable only when explicitly included by recall/skill index;
- they can be evaluated and patched, but should not silently influence all future tasks;
- promotion to `active` requires usage evidence, human approval, or configured eval pass;
- curator may consolidate quarantined skills aggressively because they are self-authored.

Lineage prevents skill explosion from becoming untraceable. A consolidated umbrella skill should record which candidates it absorbed, and absorbed candidates should point back to the umbrella skill.

## Report Shape

Skill production report should answer:

```yaml
report:
  type: skill-production
  path: user-declared|agent-offered|reflection|curator|dreaming
  mode: proposal|apply
  target: skills/example/SKILL.md
  action: create|patch|write_file|archive|consolidate
  risk: low|medium|high
  evidence:
    - reports/reflection/...
    - memory/longterm/episodic/evidence/...
  why_skill_not_memory: string
  existing_skill_search:
    searched: true
    candidates: []
    selected_target: string|null
  validation:
    schema: pass
    allowlist: pass
    protected_target: false
  provenance:
    created_by: agent
    source: reflection
    curation_policy: auto-curatable
  rollback:
    backup: backups/...
```

## Harness Mapping Of Hermes

| Hermes mechanism | Harness mapping |
|---|---|
| `~/.hermes/skills/<name>/SKILL.md` | `.mnemon/skills/**/<name>/SKILL.md` canonical artifact |
| `skills_list` / `skill_view` | skill index progressive disclosure contract |
| `skill_manage` | CLI/tool/skill write contract with create/patch/edit/write_file/archive |
| background review fork | `reflect` hook, detached review command, or queued job |
| ContextVar write origin | persisted job provenance and lineage |
| `.usage.json` | `.mnemon/state/usage.json` |
| pinned sidecar flag | `.mnemon/state/pins.json` keyed by canonical path |
| curator idle run | host scheduler, external cron, optional runner, or manual `curate` |
| `.archive/` | `.mnemon/skills/archive/` with restore metadata |

## Human Review Rules

Require human approval for:

- changes to `GUIDELINE.md`, `INSTALL.md`, `harness.yaml`;
- hook behavior changes;
- install map changes;
- evaluation policy;
- permissions and safety instructions;
- user-created or imported artifacts;
- package/core skill changes outside an upgrade flow;
- any skill that encodes external factual claims without source evidence.

## Acceptance Criteria

The skill self-evolution system is healthy when:

1. the three production entrances are distinguishable in provenance;
2. foreground user/user-confirmed skills are protected;
3. most new knowledge becomes patches or support files, not new skills;
4. one-off task details stay out of skills;
5. every skill has a clear trigger and verification path;
6. self-authored skills can be curated later;
7. user-authored/package/imported skills are protected;
8. every automated change has report, provenance, and rollback context;
9. curator improves library shape without owning the agent runtime;
10. the same design works with hooks, background review agents, runner jobs, or manual invocation.
