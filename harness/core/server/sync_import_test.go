package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

func TestRemoteMemoryImportConflictDiagnosesWithoutOverwrite(t *testing.T) {
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	rt, err := OpenSyncImportRuntime(filepath.Join(t.TempDir(), "local.db"), []contract.ResourceRef{ref})
	if err != nil {
		t.Fatalf("open sync import runtime: %v", err)
	}
	defer rt.Close()

	if err := ingestRemoteMemoryForTest(rt, "first", remoteMemoryCommitForTest(ref, "shared-entry", "remote content v1")); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	_, fields, err := rt.Resource(ref)
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "remote content v1") {
		t.Fatalf("first import did not write memory: %+v", fields)
	}

	if err := ingestRemoteMemoryForTest(rt, "conflict", remoteMemoryCommitForTest(ref, "shared-entry", "remote content v2")); err != nil {
		t.Fatalf("conflict import: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("conflict tick: %v", err)
	}
	_, fields, err = rt.Resource(ref)
	if err != nil {
		t.Fatalf("read memory after conflict: %v", err)
	}
	content, _ := fields["content"].(string)
	if strings.Contains(content, "remote content v2") || !strings.Contains(content, "remote content v1") {
		t.Fatalf("conflict import overwrote local memory: %s", content)
	}
	events, err := rt.PendingEvents(0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var diagnosed bool
	for _, ev := range events {
		if ev.Type == "remote.diagnostic" || ev.Type == "memory.diagnostic" {
			if reason, _ := ev.Payload["reason"].(string); strings.Contains(reason, "remote memory conflict") {
				diagnosed = true
			}
		}
	}
	if !diagnosed {
		t.Fatalf("conflict import must emit a durable diagnostic, events=%+v", events)
	}
}

func ingestRemoteMemoryForTest(rt *Runtime, externalID string, commit contract.LocalCommit) error {
	_, _, err := rt.API().Ingest(SyncImportActor, contract.ObservationEnvelope{
		ExternalID: externalID,
		Event: contract.Event{
			Type: RemoteMemoryCommitObserved,
			Payload: map[string]any{
				"commit": commit,
			},
		},
	})
	return err
}

func remoteMemoryCommitForTest(ref contract.ResourceRef, entryID, content string) contract.LocalCommit {
	return contract.LocalCommit{
		OriginReplicaID: "remote-replica",
		LocalDecisionID: "dec-" + entryID + "-" + strings.ReplaceAll(content, " ", "-"),
		LocalIngestSeq:  11,
		Actor:           "codex@remote",
		ResourceRef:     ref,
		ResourceVersion: 1,
		Fields: map[string]any{
			"content": "# Local Memory\n- " + content,
			"entries": []any{map[string]any{
				"id":         entryID,
				"content":    content,
				"source":     "remote",
				"confidence": "high",
				"actor":      "codex@remote",
				"ingest_seq": float64(11),
			}},
		},
		DecidedAt: "2026-06-06T00:00:00Z",
	}
}
