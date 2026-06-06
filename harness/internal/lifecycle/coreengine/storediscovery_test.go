package coreengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

// TestServerDiscoversProjectStoreFromSubdir closes the P2 adversarial store-split finding: when the
// channel server is booted from a SUBDIR of the project (a different CWD than the apply base resolved
// against the project root), it must still land on the SAME canonical store the lifecycle/app apply
// wrote to. Without project-root discovery the server's relative DefaultStorePath resolves against the
// subdir CWD -> a disjoint store -> the host pull sees absent state (the split). server.DiscoverProjectStore
// walks up to the `.mnemon` marker so both surfaces converge regardless of CWD.
func TestServerDiscoversProjectStoreFromSubdir(t *testing.T) {
	root := t.TempDir()
	ref := contract.ResourceRef{Kind: "memory", ID: "p1/e1"}

	// lifecycle/app apply writes the governed entry under the project root.
	eng := New(root, seqGen(), fixedNow())
	if res, err := eng.AdmitCreate("apply-1", "memory", string(ref.ID), map[string]any{"content": "governed", "summary": "s"}); err != nil || !res.Accepted {
		t.Fatalf("apply: %+v err=%v", res, err)
	}

	// boot the host-pull surface from a deep subdir of the project (CWD != apply base).
	sub := filepath.Join(root, "work", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	storePath := server.DiscoverProjectStore()
	rt, err := server.OpenRuntime(storePath, server.RuntimeConfig{
		Subs: map[contract.ActorID]contract.Subscription{"codex": {Actor: "codex", Refs: []contract.ResourceRef{ref}}},
	})
	if err != nil {
		t.Fatalf("open runtime at discovered store %q: %v", storePath, err)
	}
	defer rt.Close()

	proj, err := rt.API().PullProjection("codex", contract.Subscription{Actor: "codex"})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if rvVersion(proj.Resources, ref) == 0 {
		t.Fatalf("server booted from a project subdir must discover the canonical store the apply wrote to; got absent (CWD store split). discovered=%q", storePath)
	}
}
