# State Contract

State is the durable canonical record of loop-owned data.

**Canonical symbol:** canonical state lives in the kernel as versioned resources —
`contract.ResourceVersion` (per-resource `Version`, `+1` per accepted write),
persisted by `kernel.Store` and mutated ONLY through the rule pre-gate + CAS writer
(D1). The durable loop files under `.mnemon/harness/<loop>/` are the host-side
**mirror** of that canonical state, materialized by `internal/hostsurface`; source
files under `harness/loops/` are templates, not runtime state.

Every installed loop's host mirror should carry:

- `loop.json`
- `GUIDE.md`
- `env.sh`
- `status.json`
- loop-specific runtime files such as `MEMORY.md`, `skills/`, `reports/`, or
  eval artifacts
