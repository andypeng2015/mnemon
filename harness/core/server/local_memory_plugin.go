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

type wasmMemoryAdmissionRule struct {
	id      string
	actor   contract.ActorID
	emits   string
	handles map[string]bool
	guest   rule.Rule
	native  rule.Rule
}

func NewWasmMemoryAdmissionRule(ctx context.Context, principal contract.ActorID, ref contract.ResourceRef, manifest wasmrule.Manifest, wasmBytes []byte) (rule.Rule, error) {
	if _, err := wasmrule.ValidateManifest(manifest, wasmBytes); err != nil {
		return nil, err
	}
	if !containsManifestString(manifest.Emits, MemoryWriteProposed) {
		return nil, fmt.Errorf("memory admission plugin must emit %s", MemoryWriteProposed)
	}
	if !containsManifestString(manifest.Handles, MemoryWriteCandidateObserved) {
		return nil, fmt.Errorf("memory admission plugin must handle %s", MemoryWriteCandidateObserved)
	}
	guest, err := wasmrule.New(ctx, wasmBytes, wasmrule.Limits{
		Timeout:  time.Duration(manifest.Limits.TimeoutMS) * time.Millisecond,
		MemPages: uint32(manifest.Limits.MemoryPages),
	})
	if err != nil {
		return nil, err
	}
	return &wasmMemoryAdmissionRule{
		id:      "wasm-memory-admission:" + manifest.ID + ":" + string(principal),
		actor:   principal,
		emits:   MemoryWriteProposed,
		handles: map[string]bool{MemoryWriteCandidateObserved: true},
		guest:   guest,
		native:  memoryAdmissionRule(principal, ref),
	}, nil
}

func (r *wasmMemoryAdmissionRule) ID() string              { return r.id }
func (r *wasmMemoryAdmissionRule) Actor() contract.ActorID { return r.actor }
func (r *wasmMemoryAdmissionRule) Emits() string           { return r.emits }
func (r *wasmMemoryAdmissionRule) Handles(t string) bool   { return r.handles[t] }

func (r *wasmMemoryAdmissionRule) Evaluate(in rule.RuleInput) (contract.RuleDecision, error) {
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
		// The guest controls admission, not authority. Local Mnemon still stamps the canonical write from the
		// trusted event, principal, and scoped projection so a plugin cannot forge actor, type, or resource scope.
		return r.native.Evaluate(in)
	default:
		return contract.RuleDecision{Verdict: contract.VerdictAllow, Reasons: guestDecision.Reasons}, nil
	}
}

func ShadowMemoryAdmissionPlugin(ctx context.Context, principal contract.ActorID, ref contract.ResourceRef, manifest wasmrule.Manifest, wasmBytes []byte, inputs []rule.RuleInput) (rule.ShadowReport, error) {
	candidate, err := NewWasmMemoryAdmissionRule(ctx, principal, ref, manifest, wasmBytes)
	if err != nil {
		return rule.ShadowReport{}, err
	}
	native := memoryAdmissionRule(principal, ref)
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

type MemoryAdmissionPluginRegistry struct {
	fallback rule.Rule
	active   rule.Rule
}

func NewMemoryAdmissionPluginRegistry(fallback rule.Rule) *MemoryAdmissionPluginRegistry {
	return &MemoryAdmissionPluginRegistry{fallback: fallback}
}

func (r *MemoryAdmissionPluginRegistry) Active() rule.Rule {
	if r.active != nil {
		return r.active
	}
	return r.fallback
}

func (r *MemoryAdmissionPluginRegistry) Promote(ctx context.Context, principal contract.ActorID, ref contract.ResourceRef, manifest wasmrule.Manifest, wasmBytes []byte, report rule.ShadowReport) error {
	if !report.Clean {
		return fmt.Errorf("memory admission promotion rejected: shadow report not clean (%d diffs)", report.Diffs)
	}
	candidate, err := NewWasmMemoryAdmissionRule(ctx, principal, ref, manifest, wasmBytes)
	if err != nil {
		return err
	}
	r.active = candidate
	return nil
}

func (r *MemoryAdmissionPluginRegistry) Rollback() {
	r.active = nil
}

func containsManifestString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
