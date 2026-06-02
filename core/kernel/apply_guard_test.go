package kernel

import (
	"testing"

	"github.com/mnemon-dev/mnemon/core/contract"
)

// #3: an op with zero writes must NOT be committed as an Accepted no-op (it mutated nothing, so
// "accepted" is a phantom success that pollutes the decision log). It is Rejected, terminal.
func TestEmptyWritesOpIsRejected(t *testing.T) {
	k := newKernel(t)
	d := k.Apply(contract.KernelOp{OpID: "empty", Actor: "user"}, p0Modes()) // no Writes
	if d.Status == contract.Accepted {
		t.Fatal("empty op must NOT be an Accepted no-op")
	}
	if d.Status != contract.Rejected || d.NextAction != "" {
		t.Fatalf("empty op must be Rejected/'' (terminal), got %s/%q", d.Status, d.NextAction)
	}
	if k.Store().DecisionCount() != 1 {
		t.Fatalf("exactly one decision persisted, got %d", k.Store().DecisionCount())
	}
}
