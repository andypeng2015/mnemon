# Observation Contract

Observation is how Mnemon sees host reality: hook output, app-server eval
transcripts, usage evidence, reports, status files, drift, and review decisions.

Observation should be concrete enough for the reconcile path to decide whether to
act or no-op.

**Canonical symbol:** `contract.ObservationEnvelope` wrapping a `contract.Event`,
pushed into the one canonical log through the channel `server.ServerAPI.Ingest`
(D6). The host-lifecycle `schema.Event` is an envelope/payload over that canonical
event — see `internal/lifecycle/corebridge`. The kernel is the single writer; the
host pushes observations IN, never CAS-writes canonical state itself (D1).
