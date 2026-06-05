package replay

import (
	"encoding/json"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

func mref(id string) contract.ResourceRef {
	return contract.ResourceRef{Kind: "memory", ID: contract.ResourceID(id)}
}

func writesRef(ev contract.Event, id string) bool {
	b, _ := json.Marshal(ev.Payload["writes"])
	var ws []contract.ResourceWrite
	_ = json.Unmarshal(b, &ws)
	for _, w := range ws {
		if string(w.Ref.ID) == id {
			return true
		}
	}
	return false
}

// HIGH#11 (S8): the shadow diff must catch EVERY divergent decision. diffDecisions keyed decisions by OpID
// (= Event.ID, which is client-controlled and NOT unique), collapsing two decisions that share an id to the
// last one — so a candidate that denies a write the live policy accepted could be reported Clean and pass the
// Promote gate. Two proposals sharing Event.ID "dup": the candidate denies the memory/c write; Shadow MUST be
// non-clean.
func TestShadowCatchesDivergenceWhenOpIDsCollide(t *testing.T) {
	events := []contract.Event{
		proposeWrite("dup", contract.ResourceWrite{Ref: mref("c"), Kind: contract.OpCreate, Fields: map[string]any{"content": "vc"}}),
		proposeWrite("dup", contract.ResourceWrite{Ref: mref("b"), Kind: contract.OpCreate, Fields: map[string]any{"content": "vb"}}),
	}
	live := rule.RuleSet{} // permit-all
	candidate := rule.NewRuleSet(rule.NewNativeRule("denyC", "agent", "x", []string{"memory.write.proposed"},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if writesRef(in.Event, "c") {
				return contract.RuleDecision{Verdict: contract.VerdictDeny}, nil
			}
			return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
		}))
	rep := Shadow(events, live, candidate)
	if rep.Clean || rep.Diffs == 0 {
		t.Fatalf("a candidate denying a write the live policy accepted must NOT be clean even when Event.IDs collide; got %+v", rep)
	}
	// control: distinct ids, identical policy -> clean.
	control := []contract.Event{
		proposeWrite("e1", contract.ResourceWrite{Ref: mref("c"), Kind: contract.OpCreate, Fields: map[string]any{"content": "vc"}}),
		proposeWrite("e2", contract.ResourceWrite{Ref: mref("b"), Kind: contract.OpCreate, Fields: map[string]any{"content": "vb"}}),
	}
	if clean := Shadow(control, live, live); !clean.Clean {
		t.Fatalf("an identical policy must be clean; got %+v", clean)
	}
}

// MED#12 (S8/D1): sameOutcome compared Conflicts/NewVersions by LENGTH only, so a divergent conflict
// ref/version (same count) was reported equal -> a candidate that re-derives a different conflict resolution
// is falsely Clean. The comparison must cover the masked CONTENT.
func TestSameOutcomeComparesConflictContent(t *testing.T) {
	a := contract.Decision{Status: contract.Deferred, NextAction: "rebase", IngestSeq: 4,
		Conflicts: []contract.Conflict{{Ref: mref("m1"), ExpectedVersion: 1, ActualVersion: 3, Kind: contract.WriteWrite}}}
	b := contract.Decision{Status: contract.Deferred, NextAction: "rebase", IngestSeq: 4,
		Conflicts: []contract.Conflict{{Ref: mref("m1"), ExpectedVersion: 1, ActualVersion: 2, Kind: contract.WriteWrite}}}
	if sameOutcome(maskDynamic(a), maskDynamic(b)) {
		t.Fatal("same conflict COUNT but different conflict content must NOT compare equal (S8/D1)")
	}
}

func TestSameOutcomeComparesNewVersionContent(t *testing.T) {
	a := contract.Decision{Status: contract.Accepted, NewVersions: []contract.ResourceVersion{{Ref: mref("m1"), Version: 1}}}
	b := contract.Decision{Status: contract.Accepted, NewVersions: []contract.ResourceVersion{{Ref: mref("m1"), Version: 7}}}
	if sameOutcome(maskDynamic(a), maskDynamic(b)) {
		t.Fatal("same NewVersions count but different resulting versions must NOT compare equal (S8/D1)")
	}
}

// positive control: genuinely identical outcomes still compare equal (no false positive after the fix).
func TestSameOutcomeEqualWhenContentMatches(t *testing.T) {
	mk := func() contract.Decision {
		return contract.Decision{Status: contract.Accepted, IngestSeq: 2,
			NewVersions: []contract.ResourceVersion{{Ref: mref("m1"), Version: 2}, {Ref: mref("g1"), Version: 1}}}
	}
	if !sameOutcome(maskDynamic(mk()), maskDynamic(mk())) {
		t.Fatal("identical outcomes must compare equal")
	}
}
