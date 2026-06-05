// Package wasm is the wazero WASM backend behind the rule seat (D2/D10/S12). A committed .wasm rule is a PURE
// function of typed JSON input: it imports ONLY env.read_state_view (no WASI, no fs/net/clock/random — those
// host funcs are never registered, so they are structurally unavailable), it is bounded by a per-call
// deadline (WithCloseOnContextDone + context.WithTimeout — wazero has no fuel/epoch) and a memory page cap
// (WithMemoryLimitPages), and it is RETURN-ONLY: it never holds a Store/Kernel, so it can describe a decision
// but never perform a write. The same module satisfies the rule.Rule seat as the native backend.
package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	"github.com/tetratelabs/wazero"
)

// Limits bounds a wasm rule call: a per-call Timeout (wazero has NO fuel/epoch — bounding is the
// context deadline + WithCloseOnContextDone) and a memory page cap (WithMemoryLimitPages).
type Limits struct {
	Timeout  time.Duration
	MemPages uint32
}

type wasmRule struct {
	mu       sync.Mutex // the seat is shared across Ticks; serialize Evaluate
	ctx      context.Context
	runtime  wazero.Runtime
	compiled wazero.CompiledModule // compiled once; a FRESH instance is created per call (S12 purity)
	limits   Limits
	// metadata for the rule seat (fixed to the committed module's purpose; the manifest governs promotion).
	id, emits string
	actor     contract.ActorID
	handles   map[string]bool
}

// New compiles a wasm rule from module bytes. It registers ONLY the env.read_state_view host import (no WASI),
// caps memory, and enables context-deadline interruption. A throwaway instance validates the module
// instantiates WASI-free and exports memory/alloc/evaluate; the live seat then instantiates a FRESH instance
// per Evaluate (S12 purity — see evalOnce). Returns an error if the module fails to validate/instantiate (e.g.
// it imports something other than env.read_state_view, or needs WASI).
func New(ctx context.Context, wasmBytes []byte, limits Limits) (rule.Rule, error) {
	rc := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	if limits.MemPages > 0 {
		rc = rc.WithMemoryLimitPages(limits.MemPages)
	}
	rt := wazero.NewRuntimeWithConfig(ctx, rc)
	// the ONLY host import: read_state_view. No WASI, no fs/net/clock/random are ever registered.
	if _, err := rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(ptr, length uint32) uint32 { return 0 }).
		Export("read_state_view").
		Instantiate(ctx); err != nil {
		rt.Close(ctx)
		return nil, err
	}
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}
	// validate on a throwaway anonymous instance: this resolves imports (rejecting WASI / any import other than
	// env.read_state_view) and confirms the required exports, then closes immediately. WithName("") keeps it
	// anonymous so per-call instances never collide on a module name.
	probe, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}
	ok := probe.ExportedFunction("alloc") != nil && probe.ExportedFunction("evaluate") != nil && probe.Memory() != nil
	_ = probe.Close(ctx)
	if !ok {
		rt.Close(ctx)
		return nil, fmt.Errorf("wasm rule must export memory, alloc, and evaluate")
	}
	return &wasmRule{
		ctx: ctx, runtime: rt, compiled: compiled, limits: limits,
		id: "wasm-allow-if-evidence", actor: "agent", emits: "memory.write.proposed",
		handles: map[string]bool{"memory.observed": true},
	}, nil
}

func (r *wasmRule) ID() string              { return r.id }
func (r *wasmRule) Actor() contract.ActorID { return r.actor }
func (r *wasmRule) Emits() string           { return r.emits }
func (r *wasmRule) Handles(t string) bool   { return r.handles[t] }

// Evaluate runs the rule under a per-call deadline. On a runaway the deadline expires and wazero returns an
// error (never a hang). Serialized by r.mu since the seat is reused across Ticks. The module can only RETURN a
// decision (it holds no Store/Kernel — S12).
func (r *wasmRule) Evaluate(in rule.RuleInput) (contract.RuleDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.evalOnce(in)
}

// evalOnce instantiates a FRESH anonymous instance of the compiled module, runs evaluate under the per-call
// deadline, and closes the instance. A wasm rule is a PURE function of its typed input (S12): reusing one
// instance would let mutable guest globals + linear memory persist across Ticks, making even a gate-compliant
// module non-deterministic and opening a covert per-call channel. A fresh instance zeroes all guest state each
// call; a deadline kill closes only this throwaway instance, so the seat is never bricked (no reinstantiate
// dance needed).
func (r *wasmRule) evalOnce(in rule.RuleInput) (contract.RuleDecision, error) {
	inJSON, err := json.Marshal(in)
	if err != nil {
		return contract.RuleDecision{}, err
	}
	cctx, cancel := context.WithTimeout(r.ctx, r.limits.Timeout)
	defer cancel()
	mod, err := r.runtime.InstantiateModule(cctx, r.compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return contract.RuleDecision{}, err
	}
	defer mod.Close(r.ctx)
	alloc, evaluate := mod.ExportedFunction("alloc"), mod.ExportedFunction("evaluate")
	allocRes, err := alloc.Call(cctx, uint64(len(inJSON)))
	if err != nil {
		return contract.RuleDecision{}, err
	}
	ptr := uint32(allocRes[0])
	if !mod.Memory().Write(ptr, inJSON) {
		return contract.RuleDecision{}, fmt.Errorf("wasm rule: input write out of bounds")
	}
	packed, err := evaluate.Call(cctx, uint64(ptr), uint64(len(inJSON)))
	if err != nil {
		return contract.RuleDecision{}, err // deadline (sys.ExitError) or trap — surfaced, never a hang
	}
	outPtr, outLen := uint32(packed[0]>>32), uint32(packed[0])
	out, ok := mod.Memory().Read(outPtr, outLen)
	if !ok {
		return contract.RuleDecision{}, fmt.Errorf("wasm rule: output read out of bounds")
	}
	var dec contract.RuleDecision
	if err := json.Unmarshal(out, &dec); err != nil {
		return contract.RuleDecision{}, fmt.Errorf("wasm rule: decode decision: %w", err)
	}
	return dec, nil
}

// Close releases the wazero runtime.
func (r *wasmRule) Close() error { return r.runtime.Close(r.ctx) }
