package wasm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func evWith(payload map[string]any) contract.Event {
	return contract.Event{Type: "memory.observed", Payload: payload}
}

// S12: a real wazero-executed .wasm makes a real input-dependent decision (deny without evidence, propose
// with it).
func TestWasmRuleEvaluates(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, readBytes(t, "testdata/rule_allow_if_evidence.wasm"), Limits{Timeout: 50 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if d, err := r.Evaluate(rule.RuleInput{Event: evWith(nil)}); err != nil || d.Verdict != contract.VerdictDeny {
		t.Fatalf("missing evidence -> deny; got %q err=%v", d.Verdict, err)
	}
	if d, err := r.Evaluate(rule.RuleInput{Event: evWith(map[string]any{"evidence": "x"})}); err != nil || d.Verdict != contract.VerdictPropose {
		t.Fatalf("evidence -> propose; got %q err=%v", d.Verdict, err)
	}
}

// S12: a runaway module is killed by the per-call deadline (sys.ExitError-wrapped error), never a hang.
func TestWasmRunawayIsKilledByDeadline(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, readBytes(t, "testdata/loop.wasm"), Limits{Timeout: 5 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, e := r.Evaluate(rule.RuleInput{Event: evWith(nil)}); done <- e }()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("a runaway module must return a deadline error, not succeed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a runaway module must be killed by the deadline, not hang")
	}
}

// adversarial #1 (re-verify): the seat instantiates a FRESH instance per call, so a deadline kill closes only
// that throwaway instance and can never brick the long-lived seat. Every subsequent call serves the correct
// input-dependent verdict, call after call (no shared state to corrupt or recover).
func TestWasmSeatServesEveryCallIndependently(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, readBytes(t, "testdata/rule_allow_if_evidence.wasm"), Limits{Timeout: 100 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for i := 0; i < 3; i++ {
		if d, err := r.Evaluate(rule.RuleInput{Event: evWith(nil)}); err != nil || d.Verdict != contract.VerdictDeny {
			t.Fatalf("call %d (no evidence) must deny; got %q err=%v", i, d.Verdict, err)
		}
		if d, err := r.Evaluate(rule.RuleInput{Event: evWith(map[string]any{"evidence": "x"})}); err != nil || d.Verdict != contract.VerdictPropose {
			t.Fatalf("call %d (evidence) must propose; got %q err=%v", i, d.Verdict, err)
		}
	}
}

// S12: a wasm rule is a PURE function of its typed input. A gate-compliant module that carries mutable guest
// state (a global flip / linear memory) must NOT leak it across calls: identical input must yield identical
// verdicts. A reused module instance leaks the state (propose,deny,propose,...); a fresh instance per call
// resets it (propose,propose,...).
func TestWasmRuleIsDeterministicAcrossCalls(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, readBytes(t, "testdata/stateful.wasm"), Limits{Timeout: 50 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	first, err := r.Evaluate(rule.RuleInput{Event: evWith(nil)})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	for i := 0; i < 4; i++ {
		d, err := r.Evaluate(rule.RuleInput{Event: evWith(nil)})
		if err != nil {
			t.Fatalf("eval %d: %v", i, err)
		}
		if d.Verdict != first.Verdict {
			t.Fatalf("a wasm rule must be a pure fn of input; identical input gave %q then %q — mutable guest state leaked across calls (S12)", first.Verdict, d.Verdict)
		}
	}
}

// S12: the module imports only env.read_state_view -> it instantiates with NO wasi registered.
func TestWasmInstantiatesWithoutWASI(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, readBytes(t, "testdata/rule_allow_if_evidence.wasm"), Limits{Timeout: 50 * time.Millisecond, MemPages: 16}); err != nil {
		t.Fatalf("a module importing only env.read_state_view must instantiate without WASI: %v", err)
	}
}
