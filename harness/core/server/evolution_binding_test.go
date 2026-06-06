package server

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

func TestControlAgentBindingCanProposeEvolutionButNotSync(t *testing.T) {
	b := ControlAgentBinding("control@project", "http://127.0.0.1:8787", []contract.ResourceRef{{Kind: "memory", ID: "project"}})
	if !b.Allows(VerbObserve) || !b.Allows(VerbPull) || !b.Allows(VerbStatus) || !b.Allows(VerbEvolutionPropose) {
		t.Fatalf("control agent must be a normal participant that can submit evolution proposals, got %+v", b.AllowedVerbs)
	}
	if b.Allows(VerbSyncPush) || b.Allows(VerbSyncPull) || b.Allows(VerbSyncStatus) {
		t.Fatalf("control agent must not inherit sync promotion verbs, got %+v", b.AllowedVerbs)
	}
}

func TestLocalAuthorityDoesNotGrantControlAgentWrites(t *testing.T) {
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	control := ControlAgentBinding("control@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	authority := LocalAuthorityFromBindings([]ChannelBinding{control})
	if err := authority.Enforce("control@project", "memory"); err == nil {
		t.Fatal("control agents must not receive direct canonical memory write authority from local bindings")
	}
}
