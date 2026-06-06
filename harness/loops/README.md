# Mnemon Harness Loops

This directory contains canonical, host-agnostic loop templates.

```text
harness/loops/
├── memory/
└── skill/
```

Each loop follows the Loop Standard and declares its assets in
`loop.json`. Host-specific projection logic belongs under `harness/hosts/`.
The first-party product loops are memory and skill. Older non-product loop
assets are archived under `harness/experimental/archived/` for proof-only
reference and are not normal setup/install/status inputs.
