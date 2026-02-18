# Design & Philosophy

## Naming

**Mnemon** comes from Ancient Greek μνήμων (mnḗmōn), formed from μνάομαι ("to remember") with the agent suffix -μων — meaning "one who remembers." Homer uses "καὶ γὰρ μνήμων εἰμί" ("I remember well") in the Odyssey. In Greek city-states, Mnemones were official record-keepers who witnessed property transactions and legal proceedings — institutional memory carriers during the transition from oral to written tradition.

The word shares its root with Mnemosyne (Μνημοσύνη), the goddess of memory. Zeus and Mnemosyne gave birth to the nine Muses, symbolizing memory as the source of all knowledge and creation. The modern English word "mnemonic" derives from the same root.

## Core Idea: CLI-in-the-loop

Traditional memory systems (Mem0, Letta) embed LLM calls inside their pipeline — the system autonomously decides what to extract, store, and forget. This creates a dependency on API keys and adds latency and cost to every operation.

Mnemon inverts this. The binary itself contains **no LLM** — it handles storage, indexing, and graph operations. The LLM agent (Claude Code, Cursor, etc.) sits **outside** the binary and drives all intelligent operations through CLI commands:

```
┌────────────────────┐                    ┌────────────────────┐
│   LLM Agent        │   CLI commands     │     Mnemon         │
│                    │ ──────────────────► │                    │
│  - Entity extract  │   remember, recall  │  - SQLite storage  │
│  - Causal reason   │   diff, link, gc    │  - Graph indexing  │
│  - Dedup judgment  │                    │  - Keyword search  │
│  - Conflict resolve│ ◄────────────────── │  - Embedding       │
│                    │   JSON results      │  - Retention calc  │
└────────────────────┘                    └────────────────────┘
```

This is the **CLI-in-the-loop** pattern. The LLM decides **what** to remember and how to link it; the binary handles **how** to store and retrieve it. Benefits:

- **Zero API keys** — the binary never calls an LLM; all intelligence comes from the host agent
- **Stronger LLM** — the host agent (typically Opus/Sonnet) is far more capable than the small models (gpt-4o-mini) typically embedded in memory pipelines
- **Single binary** — `go install` and it works; no Python, no Docker, no external services
- **LLM-agnostic** — any tool that can execute shell commands can use mnemon

## MAGMA Four-Graph Architecture

Mnemon implements the four-graph architecture from the [MAGMA paper](https://arxiv.org/abs/2601.03236) (Multi-Graph based Agentic Memory Architecture, arXiv:2601.03236):

| Graph | Edge Type | Purpose | How Created |
|-------|-----------|---------|-------------|
| **Temporal** | `temporal` | Chronological ordering of insights | Automatic on `remember` |
| **Entity** | `entity` | Links insights sharing the same entities | Automatic via regex + LLM `--entities` flag |
| **Causal** | `causal` | Cause-effect relationships between insights | LLM creates via `link --type causal` |
| **Semantic** | `semantic` | Meaning-based similarity between insights | LLM creates via `link --type semantic` |

The first two graphs are built automatically by the binary. The latter two require LLM judgment — this is where CLI-in-the-loop shines. After each `remember`, mnemon outputs `causal_candidates` and `semantic_candidates` for the LLM to evaluate and selectively link.

### Retrieval

`recall --smart` combines multiple signals:

1. **Keyword matching** — token-based scoring
2. **Entity overlap** — shared entity edges boost relevance
3. **Graph traversal** — BFS through all four edge types with beam search
4. **Vector similarity** — cosine distance via Ollama embeddings (when available)
5. **RRF fusion** — Reciprocal Rank Fusion merges all signals into a final ranking

### Retention Lifecycle

Each insight has an `effective_importance` that decays over time:

```
effective = base_importance * log(1 + access_count) * 0.5^(days / half_life) * edge_factor
```

- **Immunity**: insights with `importance >= 4` or `access_count >= 3` are never auto-pruned
- **GC**: `mnemon gc` lists low-retention candidates for LLM review
- **Soft delete**: `forget` marks insights as deleted but preserves data

## Divergences from MAGMA

Mnemon is a practical adaptation, not a paper replica. Key differences:

| Aspect | MAGMA Paper | Mnemon |
|--------|-------------|--------|
| LLM integration | Embedded in pipeline (gpt-4o-mini) | External, CLI-in-the-loop |
| Entity extraction | LLM-based | Regex + dictionary + LLM `--entities` flag |
| Causal inference | LLM prompt chain | LLM evaluates `causal_candidates` via CLI |
| Storage | NetworkX + FAISS (in-memory) | SQLite WAL (persistent) |
| Embedding | FAISS with OpenAI | Ollama with nomic-embed-text (local, optional) |
| Deployment | Python library | Single Go binary |

For a detailed comparison, see the [architecture analysis](analysis/03-magma-paper-comparison.md).
