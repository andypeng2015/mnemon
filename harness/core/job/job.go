// Package job is the effectful job lane: external effects (a runner turn) run at-least-once under a FENCED
// lease (S5), with provider idempotency (S4) and the lease/budget as versioned kernel resources (D3/S6). The
// kernel never performs an effect — it only commits the lease/receipt/proposal a worker derives from one.
package job

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
)

type JobSpec = contract.JobSpec

// Lease is a fenced claim on a job: Fence is the lease resource's Version, the fencing token a Finish must
// match (a stale-fence Finish CANNOT overwrite a newer holder's lease, S5).
type Lease struct {
	JobID string
	Owner contract.ActorID
	Fence contract.Version
}

// Result is one runner turn's output: a durable EffectID, an Outcome, and an optional ProposalCandidate the
// lane mints into a *.proposed event (the kernel then decides it).
type Result struct {
	JobID             string
	EffectID          string
	Outcome           string
	ArtifactRefs      []string
	ProposalCandidate *contract.ProposedEvent
}

// Runner performs the actual (deterministic, in tests) external turn. Real Codex/Claude runners are a
// deferred adapter behind this interface (D6).
type Runner interface {
	Run(JobSpec) (Result, error)
}

// FakeRunner is the deterministic test runner: it records the idempotency key it saw and returns a fixed
// ProposalCandidate plus an effect id derived from the key (so a retried key yields the same effect id).
type FakeRunner struct {
	proposal *contract.ProposedEvent
	lastKey  string
	calls    int
}

func NewFakeRunner(proposal *contract.ProposedEvent) *FakeRunner { return &FakeRunner{proposal: proposal} }

func (f *FakeRunner) Run(spec JobSpec) (Result, error) {
	f.lastKey = spec.IdempotencyKey
	f.calls++
	return Result{
		JobID:             spec.IdempotencyKey,
		EffectID:          "effect_" + spec.IdempotencyKey,
		Outcome:           "ok",
		ProposalCandidate: f.proposal,
	}, nil
}
func (f *FakeRunner) LastKey() string { return f.lastKey }
func (f *FakeRunner) Calls() int      { return f.calls }

// jobModes: the lease/receipt CAS uses write_cas isolation with reject conflict mode — a lost claim/finish
// race is a hard conflict (another worker won), surfaced as an error, never a silent retry.
func jobModes() contract.Modes {
	return contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationWriteCAS, Authz: contract.AuthzStrict}
}

func leaseRef(jobID string) contract.ResourceRef {
	return contract.ResourceRef{Kind: "lease", ID: contract.ResourceID(jobID)}
}
func leaseFields(jobID string, owner contract.ActorID, fenceUntil int64) map[string]any {
	return map[string]any{"job_id": jobID, "owner": string(owner), "fence_until": float64(fenceUntil)}
}

// Claim acquires a fenced lease on jobID for owner until now+ttl. It is a read-modify-write CAS: an absent
// lease is created; an EXPIRED one (now > fence_until) or one already held by this owner is re-claimed via
// OpUpdate based_on the current version; an ACTIVE lease held by another owner is refused (S5). The resulting
// lease Version is the fence. A lost race (the CAS conflicts) surfaces as an error.
func Claim(k *kernel.Kernel, jobID string, owner contract.ActorID, now, ttl int64) (Lease, error) {
	ref := leaseRef(jobID)
	version, fields, err := k.Store().GetResource(ref)
	if err != nil {
		return Lease{}, err
	}
	fenceUntil := now + ttl
	var op contract.KernelOp
	if version == 0 {
		op = contract.KernelOp{OpID: "claim_" + jobID, Actor: owner, Writes: []contract.ResourceWrite{
			{Ref: ref, Kind: contract.OpCreate, Fields: leaseFields(jobID, owner, fenceUntil)}}}
	} else {
		curUntil := asInt64(fields["fence_until"])
		curOwner := contract.ActorID(asString(fields["owner"]))
		if now <= curUntil && curOwner != owner {
			return Lease{}, fmt.Errorf("lease %q held by %q until %d (now=%d)", jobID, curOwner, curUntil, now)
		}
		op = contract.KernelOp{OpID: "claim_" + jobID, Actor: owner, Writes: []contract.ResourceWrite{
			{Ref: ref, Kind: contract.OpUpdate, BasedOn: version, Fields: leaseFields(jobID, owner, fenceUntil)}}}
	}
	d := k.Apply(op, jobModes())
	if d.Status != contract.Accepted {
		return Lease{}, fmt.Errorf("claim %q lost the race: %s", jobID, d.Reason)
	}
	return Lease{JobID: jobID, Owner: owner, Fence: d.NewVersions[0].Version}, nil
}

// Finish releases a lease and records its effect in ONE all-or-nothing op: the lease OpUpdate is CAS'd
// based_on the held Fence (a stale fence -> the whole op is rejected, so NO receipt leaks), and the receipt
// resource is created. The lease is released by setting fence_until to now (immediately expired).
func Finish(k *kernel.Kernel, lease Lease, result Result, now int64) error {
	op := contract.KernelOp{
		OpID:  "finish_" + lease.JobID + "_" + result.EffectID,
		Actor: lease.Owner,
		Writes: []contract.ResourceWrite{
			{Ref: leaseRef(lease.JobID), Kind: contract.OpUpdate, BasedOn: lease.Fence, Fields: leaseFields(lease.JobID, lease.Owner, now)},
			{Ref: contract.ResourceRef{Kind: "receipt", ID: contract.ResourceID(result.EffectID)}, Kind: contract.OpCreate,
				Fields: map[string]any{"job_id": lease.JobID, "effect_id": result.EffectID, "outcome": result.Outcome}},
		},
	}
	d := k.Apply(op, jobModes())
	if d.Status != contract.Accepted {
		return fmt.Errorf("finish %q rejected (stale fence or duplicate effect): %s", lease.JobID, d.Reason)
	}
	return nil
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
