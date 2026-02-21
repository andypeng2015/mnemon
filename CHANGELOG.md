# Changelog

All notable changes to Mnemon will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-02-21

Initial public release.

### Added
- Core CRUD: `remember`, `recall`, `forget`, `search`, `status`, `log`
- Four-graph architecture: temporal, entity, causal, semantic edges
- Intent-aware smart recall with beam search graph traversal
- Built-in deduplication and conflict resolution
- Retention lifecycle: importance decay, access-count boosting, garbage collection
- Named memory stores for data isolation (`mnemon store list|create|set|remove`)
- `MNEMON_STORE` environment variable and `--store` CLI flag for store selection
- Automatic migration of legacy `~/.mnemon/mnemon.db` to `~/.mnemon/data/default/`
- Optional embedding support via Ollama (`nomic-embed-text`)
- Knowledge graph visualization (`mnemon viz`)
- Claude Code integration via hooks (prime, remind, nudge, compact)
- OpenClaw integration via skill deployment
- `mnemon setup` interactive installer with `--eject` support
- Release pipeline: GoReleaser, GitHub Actions, Homebrew tap
- Comprehensive documentation with Chinese translations

[Unreleased]: https://github.com/mnemon-dev/mnemon/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mnemon-dev/mnemon/releases/tag/v0.1.0
