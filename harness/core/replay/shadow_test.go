package replay

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// S8: Shadow diffs a candidate policy against the live policy over the SAME event log, WITHOUT committing
// anything to a live store/cursor. It reports diffs, never pass/fail.
func TestShadowDiffsWithoutCommitting(t *testing.T) {
	live := rule.RuleSet{} // permits everything -> all proposals applied (= the live decisions)
	// a candidate that DENIES every memory write -> the accepted writes vanish under the candidate.
	candidate := rule.NewRuleSet(rule.NewNativeRule("denier", "agent", "memory.write.proposed", []string{"memory.write.proposed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictDeny}, nil
		}))

	liveStore, _ := liveDecisions(t, sampleEvents)
	before := liveStore.DecisionCount()

	rep := Shadow(sampleEvents, live, candidate)
	if rep.Diffs == 0 || rep.Clean {
		t.Fatalf("a denying candidate must produce a non-clean diff; got %+v", rep)
	}
	if liveStore.DecisionCount() != before {
		t.Fatalf("Shadow must not mutate a live store/cursor; decision count %d -> %d", before, liveStore.DecisionCount())
	}

	// an identical candidate -> clean (zero diffs).
	clean := Shadow(sampleEvents, live, live)
	if !clean.Clean || clean.Diffs != 0 {
		t.Fatalf("an identical candidate must be clean; got %+v", clean)
	}
}
