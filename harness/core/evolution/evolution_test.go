package evolution

import "testing"

func TestControlAgentCanSubmitButCannotPromote(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}
	human := Participant{ID: "reviewer@example.com", Kind: ParticipantHumanApprover}
	proposal := EvolutionProposal{
		ID:    "plugin-memory-admission-v2",
		Kind:  ProposalPlugin,
		Stage: StageSubmitted,
		Plugin: &PluginSpec{
			ID:           "memory.admission.v2",
			Version:      "0.2.0",
			Capabilities: []string{"read_state_view"},
			Handles:      []string{"memory.write_candidate_observed"},
			Emits:        []string{"memory.write.proposed"},
		},
	}

	record, err := gov.Submit(control, proposal)
	if err != nil {
		t.Fatalf("control agent should be able to submit an evolution proposal: %v", err)
	}
	if record.Stage != StageSubmitted || record.Actor != control.ID {
		t.Fatalf("submitted proposal not recorded correctly: %+v", record)
	}
	if err := gov.Promote(control, record.ID); err == nil {
		t.Fatal("control agent must not promote directly")
	}
	if err := gov.Transition(human, record.ID, StageValidated); err != nil {
		t.Fatalf("validated: %v", err)
	}
	if err := gov.Transition(human, record.ID, StageBuilt); err != nil {
		t.Fatalf("built: %v", err)
	}
	if err := gov.Transition(human, record.ID, StageFixtureTested); err != nil {
		t.Fatalf("fixture-tested: %v", err)
	}
	if err := gov.Transition(human, record.ID, StageShadowed); err != nil {
		t.Fatalf("shadowed: %v", err)
	}
	if err := gov.Transition(human, record.ID, StageAdversarialVerified); err != nil {
		t.Fatalf("adversarial-verified: %v", err)
	}
	if err := gov.Transition(human, record.ID, StageApproved); err != nil {
		t.Fatalf("approved: %v", err)
	}
	if err := gov.Promote(human, record.ID); err != nil {
		t.Fatalf("human-approved proposal should promote: %v", err)
	}
	if active, ok := gov.Plugin("memory.admission.v2"); !ok || active.Version != "0.2.0" {
		t.Fatalf("promoted plugin missing from registry: %+v ok=%v", active, ok)
	}
}

func TestPluginProposalCannotWidenCapabilitiesSilently(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	gov.RegisterPlugin(PluginSpec{
		ID:           "memory.admission.v1",
		Version:      "0.1.0",
		Capabilities: []string{"read_state_view"},
		Handles:      []string{"memory.write_candidate_observed"},
		Emits:        []string{"memory.write.proposed"},
	})
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}

	_, err := gov.Submit(control, EvolutionProposal{
		ID:   "plugin-memory-admission-widen",
		Kind: ProposalPlugin,
		Plugin: &PluginSpec{
			ID:           "memory.admission.v1",
			Version:      "0.2.0",
			Capabilities: []string{"read_state_view", "network"},
			Handles:      []string{"memory.write_candidate_observed"},
			Emits:        []string{"memory.write.proposed"},
		},
	})
	if err == nil {
		t.Fatal("plugin proposal that silently widens capabilities must be rejected")
	}
	active, ok := gov.Plugin("memory.admission.v1")
	if !ok || active.Version != "0.1.0" || len(active.Capabilities) != 1 {
		t.Fatalf("active plugin registry changed after rejected proposal: %+v ok=%v", active, ok)
	}
}

func TestSchemaProposalCannotMakeExistingEventsAmbiguous(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	if err := gov.RegisterSchema(EventSchema{
		EventType:      "memory.write_candidate_observed",
		Version:        1,
		RequiredFields: []string{"content", "source", "confidence"},
	}); err != nil {
		t.Fatalf("register schema: %v", err)
	}
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}

	_, err := gov.Submit(control, EvolutionProposal{
		ID:   "schema-memory-ambiguous",
		Kind: ProposalSchema,
		Schema: &EventSchema{
			EventType:      "memory.write_candidate_observed",
			Version:        1,
			RequiredFields: []string{"content"},
		},
	})
	if err == nil {
		t.Fatal("schema proposal that redefines an existing event version must be rejected")
	}
}

func TestPolicyProposalCannotGrantHostAgentDirectCanonicalWrite(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}

	_, err := gov.Submit(control, EvolutionProposal{
		ID:   "policy-host-direct-write",
		Kind: ProposalPolicy,
		Policy: &PolicySpec{Grants: []PolicyGrant{{
			ActorKind:   "host-agent",
			Resource:    "memory",
			DirectWrite: true,
		}}},
	})
	if err == nil {
		t.Fatal("policy proposal must not grant HostAgent direct canonical write authority")
	}
}

func TestRejectedProposalLeavesActiveRegistryUnchanged(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	gov.RegisterPlugin(PluginSpec{
		ID:           "skill.admission.v1",
		Version:      "0.1.0",
		Capabilities: []string{"read_state_view"},
		Handles:      []string{"skill.write_candidate_observed"},
		Emits:        []string{"skill.write.proposed"},
	})
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}
	human := Participant{ID: "reviewer@example.com", Kind: ParticipantHumanApprover}
	record, err := gov.Submit(control, EvolutionProposal{
		ID:   "plugin-skill-admission-v2",
		Kind: ProposalPlugin,
		Plugin: &PluginSpec{
			ID:           "skill.admission.v1",
			Version:      "0.2.0",
			Capabilities: []string{"read_state_view"},
			Handles:      []string{"skill.write_candidate_observed"},
			Emits:        []string{"skill.write.proposed"},
		},
	})
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	if err := gov.Reject(human, record.ID, "shadow divergence"); err != nil {
		t.Fatalf("reject proposal: %v", err)
	}
	if err := gov.Promote(human, record.ID); err == nil {
		t.Fatal("rejected proposal must not promote")
	}
	active, ok := gov.Plugin("skill.admission.v1")
	if !ok || active.Version != "0.1.0" {
		t.Fatalf("active registry changed after rejection: %+v ok=%v", active, ok)
	}
}

func TestEvolutionApprovalCannotSkipGovernanceStages(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}
	human := Participant{ID: "reviewer@example.com", Kind: ParticipantHumanApprover}
	record, err := gov.Submit(control, EvolutionProposal{
		ID:   "plugin-memory-skip-stages",
		Kind: ProposalPlugin,
		Plugin: &PluginSpec{
			ID:           "memory.admission.v2",
			Version:      "0.2.0",
			Capabilities: []string{"read_state_view"},
			Handles:      []string{"memory.write_candidate_observed"},
			Emits:        []string{"memory.write.proposed"},
		},
	})
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	if err := gov.Transition(human, record.ID, StageApproved); err == nil {
		t.Fatal("approval must not skip validation, build, fixture, shadow, and adversarial verification stages")
	}
}

func TestEvolutionRollbackRestoresPriorActivePlugin(t *testing.T) {
	gov := NewGovernance(DefaultCapabilityRegistry())
	gov.RegisterPlugin(PluginSpec{
		ID:           "memory.admission.v1",
		Version:      "0.1.0",
		Capabilities: []string{"read_state_view"},
		Handles:      []string{"memory.write_candidate_observed"},
		Emits:        []string{"memory.write.proposed"},
	})
	control := Participant{ID: "control@project", Kind: ParticipantControlAgent}
	human := Participant{ID: "reviewer@example.com", Kind: ParticipantHumanApprover}
	record, err := gov.Submit(control, EvolutionProposal{
		ID:   "plugin-memory-v2",
		Kind: ProposalPlugin,
		Plugin: &PluginSpec{
			ID:           "memory.admission.v1",
			Version:      "0.2.0",
			Capabilities: []string{"read_state_view"},
			Handles:      []string{"memory.write_candidate_observed"},
			Emits:        []string{"memory.write.proposed"},
		},
	})
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	advanceToApproved(t, gov, human, record.ID)
	if err := gov.Promote(human, record.ID); err != nil {
		t.Fatalf("promote v2: %v", err)
	}
	if active, _ := gov.Plugin("memory.admission.v1"); active.Version != "0.2.0" {
		t.Fatalf("promote should activate v2, got %+v", active)
	}
	if err := gov.Rollback(human, record.ID); err != nil {
		t.Fatalf("rollback v2: %v", err)
	}
	if active, _ := gov.Plugin("memory.admission.v1"); active.Version != "0.1.0" {
		t.Fatalf("rollback should restore v1, got %+v", active)
	}
}

func advanceToApproved(t *testing.T, gov *Governance, human Participant, id string) {
	t.Helper()
	for _, stage := range []Stage{
		StageValidated,
		StageBuilt,
		StageFixtureTested,
		StageShadowed,
		StageAdversarialVerified,
		StageApproved,
	} {
		if err := gov.Transition(human, id, stage); err != nil {
			t.Fatalf("transition %s: %v", stage, err)
		}
	}
}
