package server

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/job"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/replay"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	wasmrule "github.com/mnemon-dev/mnemon/harness/core/rule/wasm"
)

// RunDemo is a runnable proof of the full control plane: it boots a ControlServer whose rule
// seat holds a REAL wazero WASM rule, drives two edges over loopback HTTP through the whole
// chain (deny/propose -> CAS -> cross-edge conflict -> scoped projection -> request_evidence
// job lane -> FakeRunner -> receipt -> proposal -> CAS -> content-tampered readback caught ->
// masked Replay), prints the decision/diagnostic/projection trace, and returns nil iff every
// link holds. It is invoked by `mnemon-harness demo` (it lived in the standalone mnemon-control
// command until the one-binary fold, D2).
func RunDemo(out io.Writer) error {
	ctx := context.Background()
	wasmBytes, err := os.ReadFile(resolveDemoWasm())
	if err != nil {
		return fmt.Errorf("read wasm rule: %w", err)
	}
	wr, err := wasmrule.New(ctx, wasmBytes, wasmrule.Limits{Timeout: 100 * time.Millisecond, MemPages: 16})
	if err != nil {
		return fmt.Errorf("instantiate wasm rule: %w", err)
	}
	fmt.Fprintln(out, "· loaded wazero WASM rule (imports only env.read_state_view, no WASI)")

	gatherRule := rule.NewNativeRule("gather", "agent", "memory.write.proposed", []string{"gather.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictRequestEvidence, Job: &contract.JobSpec{Kind: "gather", IdempotencyKey: "gather-1"}}, nil
		})

	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		return err
	}
	defer s.Close()
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{
		"agent": {"memory"}, "lane": {"lease", "receipt"},
	}})
	subs := map[contract.ActorID]contract.Subscription{
		"agent": {Actor: "agent", Refs: []contract.ResourceRef{demoRef("m1"), demoRef("m2")}},
	}
	runner := job.NewFakeRunner(&contract.ProposedEvent{Type: "memory.write.proposed", Payload: map[string]any{
		"writes": []contract.ResourceWrite{{Ref: demoRef("m2"), Kind: contract.OpCreate, Fields: map[string]any{"content": "from-runner"}}}}})

	n := 0
	newID := func() string { n++; return "id-" + strconv.Itoa(n) }
	now := func() string { return "2026-06-05T00:00:00Z" }
	modes := contract.Modes{Conflict: contract.ConflictRebase, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}
	cs := New(s, k, rule.NewRuleSet(wr, gatherRule), subs, modes, newID, now).
		WithLane(runner, "lane", func() int64 { return time.Now().Unix() }, 60)

	if _, err := s.AppendEvent(contract.Event{ID: "boot", Type: "memory.write.proposed", Actor: "agent",
		Payload: map[string]any{"writes": []contract.ResourceWrite{{Ref: demoRef("m1"), Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}}}}); err != nil {
		return err
	}
	if _, err := cs.Tick(); err != nil {
		return err
	}
	fmt.Fprintln(out, "· bootstrapped memory/m1@1")

	srv := httptest.NewServer(NewHTTPHandler(cs))
	defer srv.Close()
	edgeA := NewClient(srv.URL, "agent")
	edgeB := NewClient(srv.URL, "agent")
	obs := func(c *Client, ext, typ, corr string, payload map[string]any) error {
		_, _, err := c.Ingest("agent", contract.ObservationEnvelope{ExternalID: ext, Event: contract.Event{Type: typ, CorrelationID: corr, Payload: payload}})
		return err
	}
	for _, e := range []struct {
		c              *Client
		ext, typ, corr string
		payload        map[string]any
		note           string
	}{
		{edgeA, "a1", "memory.observed", "ca", nil, "edgeA observes (no evidence) → wasm DENY"},
		{edgeB, "b1", "memory.observed", "cb", map[string]any{"evidence": "x"}, "edgeB observes (evidence) → wasm PROPOSE m1"},
		{edgeA, "b2", "memory.observed", "cc", map[string]any{"evidence": "y"}, "edgeA observes (evidence) → wasm PROPOSE m1 (will conflict)"},
		{edgeB, "g1", "gather.observed", "cg", nil, "edgeB observes gather → request_evidence → job lane"},
	} {
		if err := obs(e.c, e.ext, e.typ, e.corr, e.payload); err != nil {
			return err
		}
		fmt.Fprintln(out, "· "+e.note)
	}

	decisions, err := cs.Tick()
	if err != nil {
		return err
	}
	var accepted, deferred int
	for _, d := range decisions {
		fmt.Fprintf(out, "  decision: %-9s op=%s %s\n", d.Status, d.OpID, d.Reason)
		switch d.Status {
		case contract.Accepted:
			accepted++
		case contract.Deferred:
			deferred++
		}
	}

	proj, err := cs.PullProjection("agent", subs["agent"])
	if err != nil {
		return err
	}
	if _, _, err := edgeB.Ingest("agent", contract.ObservationEnvelope{ExternalID: "tamper", Event: contract.Event{Type: "memory.observed", CorrelationID: "ct", ContextDigest: "tampered-" + proj.Digest, Payload: map[string]any{"evidence": "x"}}}); err != nil {
		return err
	}
	if _, err := cs.Tick(); err != nil {
		return err
	}

	evs, _ := s.PendingEvents(0)
	var stages []string
	for _, ev := range evs {
		if strings.HasSuffix(ev.Type, ".diagnostic") {
			stages = append(stages, fmt.Sprintf("%v", ev.Payload["stage"]))
		}
	}
	m1v, _ := s.GetVersion(demoRef("m1"))
	m2v, _ := s.GetVersion(demoRef("m2"))
	rv, rf, _ := s.GetResource(contract.ResourceRef{Kind: "receipt", ID: "job_k_gather-1"})
	fmt.Fprintf(out, "· diagnostics: %v\n", stages)
	fmt.Fprintf(out, "· state: memory/m1@%d  memory/m2@%d  receipt/job_k_gather-1@%d(%v)\n", m1v, m2v, rv, rf["outcome"])

	rep := replay.Replay(evs, rule.RuleSet{})
	repAccept := 0
	for _, d := range rep {
		if d.Status == contract.Accepted {
			repAccept++
		}
	}
	fmt.Fprintf(out, "· replay reproduced %d decisions (%d accepted) from the canonical log\n", len(rep), repAccept)

	switch {
	case m1v != 2:
		return fmt.Errorf("wasm propose must advance m1 to @2 via CAS, got %d", m1v)
	case m2v != 1:
		return fmt.Errorf("job lane must create m2@1, got %d", m2v)
	case rv != 1 || rf["outcome"] != "ok":
		return fmt.Errorf("job must write a receipt, got v%d %v", rv, rf)
	case accepted < 2 || deferred < 1:
		return fmt.Errorf("chain must Accept twice and Defer the conflict, got %d/%d", accepted, deferred)
	case !demoContains(stages, "rule") || !demoContains(stages, "kernel") || !demoContains(stages, "readback"):
		return fmt.Errorf("chain must surface deny(rule), conflict(kernel), and readback diagnostics, got %v", stages)
	case repAccept == 0:
		return fmt.Errorf("replay must reproduce the accepted writes")
	}
	return nil
}

func demoRef(id string) contract.ResourceRef {
	return contract.ResourceRef{Kind: "memory", ID: contract.ResourceID(id)}
}

func demoContains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// resolveDemoWasm finds the committed rule module relative to this source file (robust to
// cwd), falling back to a repo-root-relative path.
func resolveDemoWasm() string {
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		p := filepath.Join(filepath.Dir(thisFile), "..", "rule", "wasm", "testdata", "rule_allow_if_evidence.wasm")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "harness/core/rule/wasm/testdata/rule_allow_if_evidence.wasm"
}
