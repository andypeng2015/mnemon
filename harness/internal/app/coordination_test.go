package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// P3a: the AgentTeam coordination kinds (project_intent/assignment/progress_digest) are ordinary
// first-party declared kinds — they govern through the SAME assembler/appendItemRule path as
// memory/skill, with no per-kind code. This pins one (assignment, which carries the required `scope`)
// through observe → admit → resource read, plus the negative: a candidate missing the required scope
// is rejected, never written.
func TestCoordinationAssignmentGoverns(t *testing.T) {
	ref := contract.ResourceRef{Kind: "assignment", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"assignment.write_candidate.observed"}

	// nil catalog → EmbeddedCatalog, which now carries the three coordination kinds (P3a).
	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "coord.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	// positive: a well-formed assignment candidate is admitted.
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "a1",
		Event: contract.Event{Type: "assignment.write_candidate.observed", Payload: map[string]any{
			"scope": "fix projection", "ttl": "2h", "assignee": "codex@impl",
		}},
	}); err != nil {
		t.Fatalf("ingest assignment: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil || v == 0 {
		t.Fatalf("assignment must admit (v=%d err=%v)", v, err)
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "fix projection") {
		t.Fatalf("assignment content missing the candidate scope: %q", content)
	}

	// negative: scope is required (§569) — a candidate without it is rejected, version unchanged.
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "a2",
		Event: contract.Event{Type: "assignment.write_candidate.observed", Payload: map[string]any{
			"ttl": "1h", "assignee": "codex@impl",
		}},
	}); err != nil {
		t.Fatalf("ingest scopeless assignment: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v2, _, _ := rt.Resource(ref)
	if v2 != v {
		t.Fatalf("a scopeless assignment must be rejected (required scope), version moved %d -> %d", v, v2)
	}
}

// project_intent governs through the same path — a quick admit pin so all three coordination kinds
// are exercised (assignment above carries the required-field negative).
func TestCoordinationProjectIntentGoverns(t *testing.T) {
	ref := contract.ResourceRef{Kind: "project_intent", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"project_intent.write_candidate.observed"}

	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "pi.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "p1",
		Event: contract.Event{Type: "project_intent.write_candidate.observed", Payload: map[string]any{
			"statement": "ship the AgentTeam beta",
		}},
	}); err != nil {
		t.Fatalf("ingest project_intent: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil || v == 0 {
		t.Fatalf("project_intent must admit (v=%d err=%v)", v, err)
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "ship the AgentTeam beta") {
		t.Fatalf("project_intent content missing the statement: %q", content)
	}
}
