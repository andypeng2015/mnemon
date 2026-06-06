package server

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	wasmrule "github.com/mnemon-dev/mnemon/harness/core/rule/wasm"
)

type wasmSkillAdmissionRule struct {
	id      string
	actor   contract.ActorID
	emits   string
	handles map[string]bool
	guest   rule.Rule
	native  rule.Rule
}

func NewWasmSkillAdmissionRule(ctx context.Context, principal contract.ActorID, ref contract.ResourceRef, manifest wasmrule.Manifest, wasmBytes []byte) (rule.Rule, error) {
	if _, err := wasmrule.ValidateManifest(manifest, wasmBytes); err != nil {
		return nil, err
	}
	if !containsManifestString(manifest.Emits, SkillWriteProposed) {
		return nil, fmt.Errorf("skill admission plugin must emit %s", SkillWriteProposed)
	}
	if !containsManifestString(manifest.Handles, SkillWriteCandidateObserved) {
		return nil, fmt.Errorf("skill admission plugin must handle %s", SkillWriteCandidateObserved)
	}
	guest, err := wasmrule.New(ctx, wasmBytes, wasmrule.Limits{
		Timeout:  time.Duration(manifest.Limits.TimeoutMS) * time.Millisecond,
		MemPages: uint32(manifest.Limits.MemoryPages),
	})
	if err != nil {
		return nil, err
	}
	return &wasmSkillAdmissionRule{
		id:      "wasm-skill-admission:" + manifest.ID + ":" + string(principal),
		actor:   principal,
		emits:   SkillWriteProposed,
		handles: map[string]bool{SkillWriteCandidateObserved: true},
		guest:   guest,
		native:  skillAdmissionRule(principal, ref),
	}, nil
}

func (r *wasmSkillAdmissionRule) ID() string              { return r.id }
func (r *wasmSkillAdmissionRule) Actor() contract.ActorID { return r.actor }
func (r *wasmSkillAdmissionRule) Emits() string           { return r.emits }
func (r *wasmSkillAdmissionRule) Handles(t string) bool   { return r.handles[t] }

func (r *wasmSkillAdmissionRule) Evaluate(in rule.RuleInput) (contract.RuleDecision, error) {
	if in.Event.Actor != r.actor {
		return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
	}
	guestDecision, err := r.guest.Evaluate(in)
	if err != nil {
		return contract.RuleDecision{}, err
	}
	switch guestDecision.Verdict {
	case contract.VerdictDeny:
		return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: guestDecision.Reasons}, nil
	case contract.VerdictPropose:
		// The guest controls admission, not authority. Local Mnemon still stamps the append-only declaration
		// from the trusted event, principal, and scoped projection so host-native skill files remain projections.
		return r.native.Evaluate(in)
	default:
		return contract.RuleDecision{Verdict: contract.VerdictAllow, Reasons: guestDecision.Reasons}, nil
	}
}

func ShadowSkillAdmissionPlugin(ctx context.Context, principal contract.ActorID, ref contract.ResourceRef, manifest wasmrule.Manifest, wasmBytes []byte, inputs []rule.RuleInput) (rule.ShadowReport, error) {
	candidate, err := NewWasmSkillAdmissionRule(ctx, principal, ref, manifest, wasmBytes)
	if err != nil {
		return rule.ShadowReport{}, err
	}
	native := skillAdmissionRule(principal, ref)
	var diffs int
	for _, in := range inputs {
		want, _ := rule.NewRuleSet(native).Evaluate(in)
		got, _ := rule.NewRuleSet(candidate).Evaluate(in)
		if !reflect.DeepEqual(want, got) {
			diffs++
		}
	}
	return rule.ShadowReport{Clean: diffs == 0, Diffs: diffs}, nil
}

type SkillAdmissionPluginRegistry struct {
	fallback rule.Rule
	active   rule.Rule
}

func NewSkillAdmissionPluginRegistry(fallback rule.Rule) *SkillAdmissionPluginRegistry {
	return &SkillAdmissionPluginRegistry{fallback: fallback}
}

func (r *SkillAdmissionPluginRegistry) Active() rule.Rule {
	if r.active != nil {
		return r.active
	}
	return r.fallback
}

func (r *SkillAdmissionPluginRegistry) Promote(ctx context.Context, principal contract.ActorID, ref contract.ResourceRef, manifest wasmrule.Manifest, wasmBytes []byte, report rule.ShadowReport) error {
	if !report.Clean {
		return fmt.Errorf("skill admission promotion rejected: shadow report not clean (%d diffs)", report.Diffs)
	}
	candidate, err := NewWasmSkillAdmissionRule(ctx, principal, ref, manifest, wasmBytes)
	if err != nil {
		return err
	}
	r.active = candidate
	return nil
}

func (r *SkillAdmissionPluginRegistry) Rollback() {
	r.active = nil
}
