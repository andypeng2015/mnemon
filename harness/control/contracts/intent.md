# Intent Contract

Intent is the declared desired behavior for a loop on a host. It comes from:

- `harness/loops/<loop>/GUIDE.md`
- lifecycle hook prompts
- `harness/loops/<loop>/loop.json`
- `harness/bindings/<host>.<loop>.json`

Intent should be readable by the host agent without making Mnemon own host
execution.

**Canonical symbol:** the host-review wrapper is `proposal.Proposal` (Risk /
ReviewPolicy / state machine). On approval it **lowers** to a `contract.ProposedEvent`
/ `contract.KernelOp` that flows through the channel to the rule pre-gate
(`rule.Rule` / `rule.RuleSet`) → bridge → `kernel.Apply` — the kernel is the only
writer (D1). See `internal/lifecycle/coreengine` for the memory/eval/coordination
lowerings.
