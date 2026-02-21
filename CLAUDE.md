# Mnemon — Project Guidelines

## Development

- **Build**: `go build -o mnemon .`
- **Install**: `make setup` (binary + skill + hooks)
- **Test**: `bash scripts/e2e_test.sh`
- **Dependencies**: `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`
- **Optional**: Ollama with `nomic-embed-text` for embedding support
