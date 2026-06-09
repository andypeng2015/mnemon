package assembler

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// A 3rd capability (note) stands up end-to-end through config + the generic kind alone — no new rule
// code: Assemble compiles the config into a runtime config whose note rule admits a note candidate
// through the channel -> tick -> kernel -> projection.
func TestAssembleAdmitsConfiguredNoteCapabilityEndToEnd(t *testing.T) {
	ref := contract.ResourceRef{Kind: "note", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"note.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"note": {Enabled: true, ResourceRef: "note/project", RuleRef: "native:note"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "n1",
		Event:      contract.Event{Type: "note.write_candidate.observed", Payload: map[string]any{"text": "remember the assembler"}},
	}); err != nil {
		t.Fatalf("ingest note: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if v == 0 {
		t.Fatal("the configured note capability must admit a candidate (resource not created)")
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "remember the assembler") {
		t.Fatalf("note content missing the candidate: %q", content)
	}
}

func TestAssembleFailsClosedOnUnknownCapability(t *testing.T) {
	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"bogus": {Enabled: true, ResourceRef: "bogus/project", RuleRef: "native:bogus"},
	}}
	if _, err := Assemble(cfg, nil); err == nil {
		t.Fatal("an unknown capability rule_ref must fail closed")
	}
}

// A binding scoped to a non-default ref of the capability's kind must get a rule targeting ITS ref
// (parity with the production memoryRefForBinding fallback), not the config-pinned default.
func TestAssembleDerivesRefFromBindingScope(t *testing.T) {
	teamRef := contract.ResourceRef{Kind: "memory", ID: "team"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{teamRef})
	binding.AllowedObservedTypes = []string{"memory.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"memory": {Enabled: true, ResourceRef: "memory/project", RuleRef: "native:memory"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event:      contract.Event{Type: "memory.write_candidate.observed", Payload: map[string]any{"content": "team fact", "source": "s", "confidence": "high"}},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, err := rt.Resource(teamRef); err != nil || v == 0 {
		t.Fatalf("write must land on the binding's scoped ref memory/team (v=%d err=%v)", v, err)
	}
	if v, _, _ := rt.Resource(contract.ResourceRef{Kind: "memory", ID: "project"}); v != 0 {
		t.Fatal("the config default memory/project must NOT be written for a team-scoped binding")
	}
}

// A host-agent binding with observe + observed-type but EMPTY SubscriptionScope must produce no rule
// and no kernel authority (parity with the app builders' skip; an unscoped binding could never pull
// what it writes).
func TestAssembleSkipsUnscopedBinding(t *testing.T) {
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", nil)
	binding.AllowedObservedTypes = []string{"memory.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"memory": {Enabled: true, ResourceRef: "memory/project", RuleRef: "native:memory"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got := len(rc.Authority.Allow["codex@project"]); got != 0 {
		t.Fatalf("unscoped binding must get no kernel authority, got %d kinds", got)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event:      contract.Event{Type: "memory.write_candidate.observed", Payload: map[string]any{"content": "x", "source": "s", "confidence": "high"}},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, _ := rt.Resource(contract.ResourceRef{Kind: "memory", ID: "project"}); v != 0 {
		t.Fatal("an unscoped binding must not produce a write")
	}
}
