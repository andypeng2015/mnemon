# Projection Contract

Projection is the host-readable view generated from canonical state and binding
intent. Projection files live under host-owned directories such as `.codex` or
`.claude` and must be treated as generated views.

Projection must not become a second source of truth.

**Canonical symbols (the word `projection` is split by ring):**

- Kernel projection — `core/projection.Projection` (scoped read-set + content
  digest), pulled out through the channel `server.ServerAPI.PullProjection` (D6).
- Host surface — `internal/hostsurface` writes the `.codex` / `.claude` files. It
  is a MIRROR of canonical state, never an independent writer.

`projection` is reserved for the kernel; the host writer is `hostsurface` (D4).
