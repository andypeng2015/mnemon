package runtime

import (
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/job"
)

// The job lane is gated on the runtime config: a nil Runner leaves it OFF (a job verdict is inert);
// a configured Runner + LaneOwner + LaneTTL wires it on.
func TestRuntimeLaneGatedOnRunner(t *testing.T) {
	off, err := OpenRuntime(filepath.Join(t.TempDir(), "off.db"), RuntimeConfig{})
	if err != nil {
		t.Fatalf("open (no lane): %v", err)
	}
	defer off.Close()
	if off.cs.runner != nil {
		t.Fatal("a nil Runner must leave the job lane unconfigured")
	}

	on, err := OpenRuntime(filepath.Join(t.TempDir(), "on.db"), RuntimeConfig{
		Runner: job.NewFakeRunner(nil), LaneOwner: "lane", LaneTTL: 60,
	})
	if err != nil {
		t.Fatalf("open (lane): %v", err)
	}
	defer on.Close()
	if on.cs.runner == nil || on.cs.laneOwner != "lane" || on.cs.laneTTL != 60 {
		t.Fatalf("a configured Runner must wire the lane; runner=%v owner=%q ttl=%d", on.cs.runner != nil, on.cs.laneOwner, on.cs.laneTTL)
	}
}
