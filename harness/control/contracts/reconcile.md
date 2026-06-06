# Reconcile Contract

Reconcile compares Intent with Reality and writes the result back to State.

**Canonical symbol:** `core/reconcile` is the CAS decider — it decides pending
`*.proposed` events against the canonical read-set and conflict/isolation/authz
modes (`reconcile.ResolveModes`), and the kernel is the sole writer. The host-side
fold that materializes the read model from the event log is the **projection fold**
`internal/lifecycle/status` (and `coordination.DeriveView`) — a fold, not a writer.

Host-side reconcile paths that remain procedural:

- host projectors (`internal/hostsurface`) install and refresh the host surface
- protocol skills record online evidence or apply approved changes
- maintenance agents curate, consolidate, or propose changes

These consume `loop.json`, `host.json`, `bindings/*.json`, host manifests, and
loop `status.json`.
