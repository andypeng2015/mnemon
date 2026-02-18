# Mnemon — Project Guidelines

## What is this project

Mnemon is a persistent memory system for LLM agents, built in Go with SQLite storage and a MAGMA-aligned four-graph architecture (temporal, entity, causal, semantic edges).

## Memory Skill

The mnemon skill template is at `skills/mnemon/SKILL.md`. Run `make setup` to install it globally to `~/.claude/skills/mnemon/`.

## Development

- **Build**: `go build -o mnemon .`
- **Install**: `go build -o $GOPATH/bin/mnemon .`
- **Test**: `./scripts/e2e_test.sh`
- **Dependencies**: `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`
- **Optional**: Ollama with `nomic-embed-text` for embedding support
