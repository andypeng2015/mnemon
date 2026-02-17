# Mnemon

An open-source memory daemon for LLM agents.

## What is Mnemon?

Mnemon is a standalone, persistent memory service that gives LLM agents reliable long-term memory. It runs as an independent daemon — not a library, not a plugin — accessible by any LLM-CLI, Web UI, or agent over standard interfaces.

**Core problem**: LLM agents forget everything between sessions. Context compaction drops critical decisions, cross-session knowledge is lost, and long conversations push early information out of the window. Mnemon solves this by providing a shared, graph-native memory layer that persists independently of any single agent session.

## Key Ideas

- **Independent daemon** — 24/7 process, not tied to any agent's lifecycle
- **Graph-native storage** — MAGMA 4-graph architecture (semantic, temporal, causal, entity edges), not just vector similarity
- **LLM-driven memory management** — the LLM actively decides what to remember, update, or forget
- **Multi-agent maintenance** — dedicated agents can concurrently organize, verify, and enrich the memory graph
- **LLM-agnostic** — works with any LLM-CLI (Claude Code, Cursor, etc.) via Skills + a single binary

## Architecture

```
[Claude Code] ──skill──→ [Mnemon Daemon] ←──REST API──── [Web UI]
[Cursor]      ──skill──→ [Mnemon Daemon] ←──CLI────────── [Developer]
[Other Agent] ──skill──→ [Mnemon Daemon] ←──Agent─────── [Maintenance Agent]
```

## Documentation

Detailed design documents are in [`docs/`](docs/00-vision.md):

| Doc | Topic |
|-----|-------|
| [00 Vision](docs/00-vision.md) | Project vision & naming |
| [01 Problem](docs/01-problem.md) | Why agents need persistent memory |
| [02 Philosophy](docs/02-philosophy.md) | Design philosophy: LLM-CLI as game engine |
| [03 Landscape](docs/03-landscape.md) | Existing solutions & comparison |
| [04 Mem0 Analysis](docs/04-mem0-analysis.md) | Mem0 technical deep dive |
| [05 MemCP Analysis](docs/05-memcp-analysis.md) | MemCP technical deep dive |
| [06 Architecture](docs/06-architecture.md) | Core architecture design |
| [07 Memory Strategy](docs/07-memory-strategy.md) | Collection strategy: wide intake, strict output |
| [08 Cowork](docs/08-cowork.md) | Dual-instance async architecture |
| [09 Multi-Agent](docs/09-multi-agent.md) | Multi-agent memory maintenance |
| [10 RAG Comparison](docs/10-rag-comparison.md) | Positioning vs RAG ecosystem |
| [11 Community](docs/11-community.md) | Community consensus & market validation |
| [12 Vector & Pipeline](docs/12-vector-and-pipeline.md) | Vector layer & learning pipeline |
| [13 Binary Evolution](docs/13-binary-evolution.md) | Architecture evolution: Binary + Skill |
| [14 Decentralization](docs/14-decentralization.md) | Decentralization analysis |
| [15 Academic](docs/15-academic.md) | Academic foundations |
| [16 Implementation](docs/16-implementation.md) | Roadmap & TODOs |
| [17 References](docs/17-references.md) | References |

## Status

Early design phase. See [implementation roadmap](docs/16-implementation.md) for planned milestones.

## License

[GPL-3.0](LICENSE)
