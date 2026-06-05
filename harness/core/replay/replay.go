// Package replay re-derives decisions from the canonical event log on a throwaway kernel (event-sourcing
// purity, S8): replay reads the log only, never advances a live cursor or writes a live store, and its
// determinism is established by FIELD-MASKING the dynamic decision fields (DecisionID/AppliedAt) before any
// diff (D1) — production decisions keep their real uuid/time. replay imports rule (one-way, D11).
package replay

import (
	"sort"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/reconcile"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// canonicalModes is the fixed policy replay reconciles under; it matches the server's loop modes so a replay
// reproduces the live decisions deterministically.
var canonicalModes = contract.Modes{Conflict: contract.ConflictRebase, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}

func isProposal(ev contract.Event) bool { return strings.HasSuffix(ev.Type, ".proposed") }

// permissiveAuthority lets every actor that appears in the events write every catalog kind, so replay does
// not introduce authz rejections the live run did not have (the live authority is reproduced by the events
// themselves having been accepted; replay only re-derives, it does not re-police).
func permissiveAuthority(events []contract.Event) kernel.AuthorityRules {
	var kinds []contract.ResourceKind
	for k := range contract.KindCatalog {
		kinds = append(kinds, k)
	}
	allow := map[contract.ActorID][]contract.ResourceKind{}
	for _, ev := range events {
		if _, ok := allow[ev.Actor]; !ok {
			allow[ev.Actor] = kinds
		}
	}
	return kernel.AuthorityRules{Allow: allow}
}

// Replay re-derives the decisions by reconciling the *.proposed events of the log over a FRESH :memory:
// kernel. It is a pure function of the events (no live store), reproducing the live decisions up to the
// masked dynamic fields. The candidate ruleset is retained for signature symmetry with Shadow — pure replay
// needs no policy because the logged proposals are authoritative (event-sourcing).
func Replay(events []contract.Event, candidate rule.RuleSet) []contract.Decision {
	return drive(events, nil)
}

// Shadow replays the same event log under the LIVE and the CANDIDATE policies (each on its own throwaway
// kernel) and reports the diff — never committing to a live store or advancing a cursor (S8). A candidate
// that denies writes the live policy accepted yields a non-clean report; an identical candidate is clean. It
// reports diffs, never pass/fail (the operator gates promotion on Clean).
func Shadow(events []contract.Event, live, candidate rule.RuleSet) rule.ShadowReport {
	liveDecs := drive(events, &live)
	candDecs := drive(events, &candidate)
	diffs := diffDecisions(liveDecs, candDecs)
	return rule.ShadowReport{Clean: diffs == 0, Diffs: diffs}
}

// diffDecisions counts the decisions that differ between two replays, keyed by the durable IngestSeq and
// compared on the masked, outcome-bearing fields (a missing or differing decision on either side is one diff).
// The key is IngestSeq — the event's autoincrement rowid, unique per decision — NOT OpID (= the
// client-controllable Event.ID, which can collide): keying by a non-unique field collapsed two decisions that
// shared an id to the last one (last-write-wins), hiding a real divergence and producing a FALSE-CLEAN report
// the Promote gate trusts (S8).
func diffDecisions(a, b []contract.Decision) int {
	index := func(ds []contract.Decision) map[int64]contract.Decision {
		m := make(map[int64]contract.Decision, len(ds))
		for _, d := range ds {
			m[d.IngestSeq] = maskDynamic(d)
		}
		return m
	}
	am, bm := index(a), index(b)
	diffs := 0
	for seq, ad := range am {
		if bd, ok := bm[seq]; !ok || !sameOutcome(ad, bd) {
			diffs++
		}
	}
	for seq := range bm {
		if _, ok := am[seq]; !ok {
			diffs++
		}
	}
	return diffs
}

// sameOutcome compares the masked, non-dynamic decision fields by CONTENT — not just the COUNT of
// Conflicts/NewVersions. A length-only compare reported a candidate that re-derived a divergent conflict
// (different raced ref/version) or a different resulting version as equal, defeating the S8/D1 equivalence the
// Clean gate provides. maskDynamic sorts both slices, so the element-wise compare is order-insensitive.
func sameOutcome(a, b contract.Decision) bool {
	if a.Status != b.Status || a.NextAction != b.NextAction || a.IngestSeq != b.IngestSeq {
		return false
	}
	if len(a.Conflicts) != len(b.Conflicts) || len(a.NewVersions) != len(b.NewVersions) {
		return false
	}
	for i := range a.Conflicts {
		if a.Conflicts[i] != b.Conflicts[i] {
			return false
		}
	}
	for i := range a.NewVersions {
		if a.NewVersions[i] != b.NewVersions[i] {
			return false
		}
	}
	return true
}

// drive replays the events on a throwaway kernel and returns the reconciler's decisions. If filter is
// non-nil, a *.proposed event the filter would DENY is neutralized (re-typed so the reconciler skips it,
// preserving every other event's durable seq) — this is how Shadow diffs a candidate policy without re-
// ordering the log.
func drive(events []contract.Event, filter *rule.RuleSet) []contract.Decision {
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		return nil
	}
	defer s.Close()
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), permissiveAuthority(events))
	r := reconcile.NewReconciler(s, k)
	for _, ev := range events {
		e := ev
		if filter != nil && isProposal(ev) {
			dec, _ := filter.Evaluate(rule.RuleInput{Event: ev})
			if dec.Verdict == contract.VerdictDeny {
				e.Type = ev.Type + ".shadow_denied" // not a proposal -> reconciler skips; seq preserved
			}
		}
		if _, err := s.AppendEvent(e); err != nil {
			continue
		}
	}
	return r.RunOnce(canonicalModes)
}

// maskDynamic zeros the per-run dynamic fields and sorts the order-insensitive slices so two decisions for
// the same logical outcome compare equal regardless of uuid/time/ordering (D1).
func maskDynamic(d contract.Decision) contract.Decision {
	d.DecisionID = ""
	d.AppliedAt = ""
	sort.Slice(d.Conflicts, func(i, j int) bool {
		return string(d.Conflicts[i].Ref.Kind)+string(d.Conflicts[i].Ref.ID) < string(d.Conflicts[j].Ref.Kind)+string(d.Conflicts[j].Ref.ID)
	})
	sort.Slice(d.NewVersions, func(i, j int) bool {
		return string(d.NewVersions[i].Ref.Kind)+string(d.NewVersions[i].Ref.ID) < string(d.NewVersions[j].Ref.Kind)+string(d.NewVersions[j].Ref.ID)
	})
	return d
}
