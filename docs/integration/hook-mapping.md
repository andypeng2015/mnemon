# Hook Mapping: Claude Code ↔ OpenClaw

> Research date: 2026-02-19
>
> Claude Code docs: [code.claude.com/docs/en/hooks](https://code.claude.com/docs/en/hooks)
> OpenClaw docs: [docs.openclaw.ai/automation/hooks](https://docs.openclaw.ai/automation/hooks)
> OpenClaw plugin PR: [#14882](https://github.com/openclaw/openclaw/pull/14882)
> OpenClaw lifecycle PR: [#18889](https://github.com/openclaw/openclaw/pull/18889) (pending)

## Architecture Comparison

| | Claude Code | OpenClaw |
|--|-------------|----------|
| **Hook systems** | 1 (unified) | 2 (directory-based + plugin-based) |
| **Handler types** | 3: `command` (shell), `prompt` (LLM), `agent` (multi-turn LLM) | Directory: TypeScript `handler.ts`; Plugin: `api.on()` TypeScript module |
| **IO model** | stdin JSON → stdout JSON/text | Event object → `event.messages.push()` (directory) or callback payload (plugin) |
| **Registration** | `settings.json` (`hooks.<Event>`) | Directory: auto-discovery from `~/.openclaw/hooks/` + `config.json`; Plugin: `register(api)` |
| **Blocking** | Exit code 2 or `decision: "block"` JSON | Directory: cannot block; Plugin: only `message_sending` can modify/cancel |
| **Matcher/filter** | Regex on tool name, session source, etc. | YAML `events` list in `HOOK.md` frontmatter |
| **Async support** | `"async": true` on command hooks | Plugin hooks are inherently async (fire-and-forget or awaited) |
| **Snapshot security** | Hooks frozen at startup; mid-session edits require review | No equivalent protection |
| **Total events** | 14 | Directory: 8; Plugin: 14 (+ 6 pending in PR #18889) |

## Event Mapping by Lifecycle Phase

### Session Lifecycle

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| Session begins | `SessionStart` | `gateway:startup`, `command:new` | `gateway_start`, `session_start` | CC has single event; OC splits into gateway vs session |
| Session ends | `SessionEnd` | — | `session_end` | Directory hooks cannot observe session end |
| Environment setup | `SessionStart` + `$CLAUDE_ENV_FILE` | `agent:bootstrap` (mutable `bootstrapFiles`) | — | CC persists env vars; OC injects files into prompt |

**Detail:**

| | Claude Code `SessionStart` | OpenClaw `gateway:startup` / `command:new` |
|--|---|---|
| **Trigger** | Session begins or resumes | `gateway:startup`: after gateway boots; `command:new`: user issues `/new` |
| **Input** | `{ source, model, agent_type }` | `gateway:startup`: none; `command:new`: `{ sessionEntry }` |
| **Output** | stdout → context for Claude; can write to `$CLAUDE_ENV_FILE` | `event.messages.push()` → sent to user |
| **Can block** | No | No |
| **Matcher** | `startup`, `resume`, `clear`, `compact` | — |

| | Claude Code `SessionEnd` | OpenClaw `session_end` (plugin) |
|--|---|---|
| **Trigger** | Session terminates | Session replaced by new session |
| **Input** | `{ reason }` | `{ sessionId, messageCount, durationMs }` |
| **Output** | Cannot block | Cannot block |
| **Matcher** | `clear`, `logout`, `prompt_input_exit`, etc. | — |

### User Input

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| User sends message | `UserPromptSubmit` | `message:received` | `message_received` | CC is agent-level; OC is gateway-level (from any channel) |

**Detail:**

| | Claude Code `UserPromptSubmit` | OpenClaw `message:received` |
|--|---|---|
| **Trigger** | User submits prompt, before Claude processes it | Inbound message arrives from any channel |
| **Input** | `{ prompt }` | `{ from, content, channelId }` |
| **Output** | stdout/`additionalContext` → context for Claude | `event.messages.push()` → sent to user/model |
| **Can block** | Yes (`decision: "block"` or exit 2) | No |
| **Semantic difference** | "Prompt about to enter the model" | "Message entered the system from a channel" |

### Tool Execution

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| Before tool call | `PreToolUse` | — | `before_tool_call` | No directory-based equivalent |
| Permission dialog | `PermissionRequest` | — | — | OC has no permission system hooks |
| After tool success | `PostToolUse` | — | `after_tool_call` | No directory-based equivalent |
| After tool failure | `PostToolUseFailure` | — | (same `after_tool_call` with error) | CC distinguishes success/failure; OC unifies |
| Tool result transform | — | — | `tool_result_persist` (sync, can modify) | CC has no equivalent; OC can rewrite tool output |

**Detail:**

| | Claude Code `PreToolUse` | OpenClaw `before_tool_call` (plugin) |
|--|---|---|
| **Trigger** | After Claude creates tool params, before execution | Before tool invocation |
| **Input** | `{ tool_name, tool_input, tool_use_id }` | Tool name, params |
| **Output** | `permissionDecision` (allow/deny/ask), `updatedInput` | Observe only (fire-and-forget) |
| **Can block** | Yes (deny tool call) | No |
| **Can modify** | Yes (`updatedInput` rewrites tool params) | No |
| **Matcher** | Tool name regex: `Bash`, `Edit\|Write`, `mcp__.*` | — |

| | Claude Code `PostToolUse` | OpenClaw `after_tool_call` (plugin) |
|--|---|---|
| **Trigger** | After tool completes successfully | After tool execution completes |
| **Input** | `{ tool_name, tool_input, tool_response, tool_use_id }` | `{ tool params, result, error, durationMs }` |
| **Output** | `additionalContext`, `decision: "block"` | Observe only |
| **Can block** | Yes (blocks further processing) | No |

| | Claude Code — (none) | OpenClaw `tool_result_persist` (plugin) |
|--|---|---|
| **Trigger** | — | Before tool result written to transcript |
| **Input** | — | Tool result payload |
| **Output** | — | **Sync transform**: can rewrite tool result |
| **Unique to** | — | OpenClaw only |

### Agent Response

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| Agent finishes responding | `Stop` | — | `agent_end` | **Critical gap**: no directory-based equivalent in OC |
| Agent response start | — | — | — | Pending: `agent:response:start` (PR #18889) |
| Agent response end | — | — | — | Pending: `agent:response:end` (PR #18889) |
| Agent thinking start | — | — | — | Pending: `agent:thinking:start` (PR #18889) |
| Agent thinking end | — | — | — | Pending: `agent:thinking:end` (PR #18889) |

**Detail:**

| | Claude Code `Stop` | OpenClaw `agent_end` (plugin) |
|--|---|---|
| **Trigger** | Main agent finishes responding (not on user interrupt) | Agent turn completed |
| **Input** | `{ stop_hook_active, last_assistant_message }` | Final message list + run metadata |
| **Output** | `decision: "block"` forces continuation | Observe only (fire-and-forget) |
| **Can block** | Yes (prevents stopping, continues conversation) | No |
| **Loop prevention** | `stop_hook_active` flag | N/A |
| **Handler type** | Shell script, prompt, or agent | TypeScript plugin module only |

**Note:** `command:stop` in OpenClaw is user-initiated interruption (`/stop`), NOT "agent finished responding". It is the semantic opposite of Claude Code's `Stop`.

### Outbound Message

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| Before message delivery | — | — | `message_sending` (can modify/cancel, awaited) | CC has no outbound pre-hook |
| After message delivery | — | `message:sent` (v2026.2.19) | `message_sent` | CC has no outbound post-hook |

### Context Compaction

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| Before compaction | `PreCompact` | — | `before_compaction` | No directory-based equivalent in OC |
| After compaction | — | — | `after_compaction` | CC has no post-compaction hook |

**Detail:**

| | Claude Code `PreCompact` | OpenClaw `before_compaction` (plugin) |
|--|---|---|
| **Trigger** | Before context compaction | Before auto-compaction starts |
| **Input** | `{ trigger: "manual"\|"auto", custom_instructions }` | None |
| **Output** | Cannot block | Cannot block (fire-and-forget) |
| **Matcher** | `manual`, `auto` | — |

### Subagent / Teammate

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| Subagent spawned | `SubagentStart` | — | — | OC has no subagent concept |
| Subagent finished | `SubagentStop` | — | — | OC has no subagent concept |
| Teammate going idle | `TeammateIdle` | — | — | OC has no multi-agent team concept |
| Task completed | `TaskCompleted` | — | — | OC has no task completion hook |

### Notifications

| Phase | Claude Code | OpenClaw (Directory) | OpenClaw (Plugin) | Gap |
|-------|-------------|---------------------|-------------------|-----|
| System notification | `Notification` | — | — | OC has no notification hook |

## Capability Comparison

### Blocking (prevent action from proceeding)

| Capability | Claude Code | OpenClaw |
|-----------|-------------|----------|
| Block user prompt | `UserPromptSubmit` exit 2 or `decision: "block"` | Not possible |
| Block tool call | `PreToolUse` → `permissionDecision: "deny"` | Not possible |
| Block agent stop | `Stop` → `decision: "block"` | Not possible |
| Block message delivery | Not possible | `message_sending` plugin (awaited, can cancel) |

### Modification (rewrite data before it proceeds)

| Capability | Claude Code | OpenClaw |
|-----------|-------------|----------|
| Modify tool input | `PreToolUse` → `updatedInput` | Not possible |
| Modify tool output | — | `tool_result_persist` plugin (sync transform) |
| Modify outbound message | — | `message_sending` plugin (can rewrite content) |
| Inject context | `UserPromptSubmit` / `SessionStart` stdout → Claude context | `message:received` → `event.messages.push()` |
| Inject bootstrap files | — | `agent:bootstrap` (mutable `bootstrapFiles`) |

### Observation (read-only)

Both systems support observation across all events. Claude Code shows hook output in verbose mode (`Ctrl+O`). OpenClaw plugin hooks log errors but never block core operations.

## Pending Changes

### OpenClaw PR #18889 (agent lifecycle hooks for directory system)

Would add to directory-based hooks:

| New Event | Maps To (Claude Code) |
|-----------|-----------------------|
| `agent:response:end` | `Stop` (closest equivalent) |
| `agent:response:start` | — (no CC equivalent) |
| `agent:thinking:start` / `end` | — (no CC equivalent) |
| `agent:tool:start` / `end` | `PreToolUse` / `PostToolUse` (observation only) |

**Status:** Open, awaiting maintainer review. Would close issues #7724, #7597, #5513.

### OpenClaw Community Proposal (extended hook system)

[Gist by openmetaloom](https://gist.github.com/openmetaloom/657c4668c09d235f8da1306e2438904b) proposes:

| Proposed Phase | Maps To (Claude Code) |
|---------------|-----------------------|
| `preRequest` | `UserPromptSubmit` |
| `preRecall` | — (memory-specific, no CC equivalent) |
| `preResponse` | — (no CC equivalent; CC uses `Stop` post-response) |
| `postResponse` | `Stop` |
| `preToolExecution` / `postToolExecution` | `PreToolUse` / `PostToolUse` |
| `preCompaction` / `postCompaction` | `PreCompact` / — |

**Status:** Proposal only, not officially adopted.

## Summary: Coverage Matrix

✅ = available, ⚠️ = partial/workaround, ❌ = not available

| Lifecycle Phase | Claude Code | OpenClaw Directory | OpenClaw Plugin |
|----------------|-------------|-------------------|-----------------|
| Session start | ✅ `SessionStart` | ✅ `gateway:startup` / `command:new` | ✅ `gateway_start` / `session_start` |
| Session end | ✅ `SessionEnd` | ❌ | ✅ `session_end` |
| User input | ✅ `UserPromptSubmit` (blockable) | ⚠️ `message:received` (observe only) | ⚠️ `message_received` (observe only) |
| Pre tool call | ✅ `PreToolUse` (block/modify) | ❌ | ⚠️ `before_tool_call` (observe only) |
| Permission | ✅ `PermissionRequest` (allow/deny) | ❌ | ❌ |
| Post tool call | ✅ `PostToolUse` | ❌ | ✅ `after_tool_call` |
| Tool failure | ✅ `PostToolUseFailure` | ❌ | ⚠️ (same `after_tool_call`) |
| Tool result rewrite | ❌ | ❌ | ✅ `tool_result_persist` |
| Agent response done | ✅ `Stop` (blockable) | ❌ | ⚠️ `agent_end` (observe only) |
| Outbound pre-send | ❌ | ❌ | ✅ `message_sending` (modify/cancel) |
| Outbound post-send | ❌ | ✅ `message:sent` | ✅ `message_sent` |
| Pre compaction | ✅ `PreCompact` | ❌ | ✅ `before_compaction` |
| Post compaction | ❌ | ❌ | ✅ `after_compaction` |
| Bootstrap/prompt inject | ⚠️ `SessionStart` stdout | ✅ `agent:bootstrap` (mutable) | ❌ |
| Subagent lifecycle | ✅ `SubagentStart` / `SubagentStop` | ❌ | ❌ |
| Multi-agent team | ✅ `TeammateIdle` / `TaskCompleted` | ❌ | ❌ |
| Notification | ✅ `Notification` | ❌ | ❌ |
