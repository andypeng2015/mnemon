package hostsurface

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --dry-run must be accepted by the claude-code projector and write nothing (the codex projector has
// supported it since the diff engine landed; claude previously hard-failed on the option).
func TestClaudeProjectorDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
		HostArgs:    []string{"--dry-run"},
		Stdout:      &out,
	})
	if err != nil {
		t.Fatalf("dry-run must be accepted: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not create the projection surface")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".mnemon")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not create harness state")
	}
	if !strings.Contains(out.String(), "would write") {
		t.Fatalf("dry-run must report per-file would-write lines, got: %q", out.String())
	}
}

func TestClaudeProjectorReportDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	rep, err := RunClaudeProjectorReport(context.Background(), ClaudeOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
		HostArgs:    []string{"--dry-run"},
	})
	if err != nil {
		t.Fatalf("dry-run must be accepted: %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Fatalf("dry-run report must be empty, got %v", rep.Conflicts)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not create the projection surface")
	}
}

// The dry-run report must come from the REAL no-clobber classifier: a user-edited managed file is
// reported as "would preserve", never "would update" (the lie the deleted desired-files diff model
// told — it raw-byte-compared and claimed updates the real install would refuse).
func TestCodexDryRunReportsPreserveForUserEditedFile(t *testing.T) {
	dir := t.TempDir()
	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	guide := filepath.Join(dir, ".codex", "mnemon-memory", "GUIDE.md")
	if err := os.WriteFile(guide, []byte("# USER EDIT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
		HostArgs:    []string{"--dry-run"},
		Stdout:      &out,
	}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	report := out.String()
	if !strings.Contains(report, "would preserve user-modified .codex/mnemon-memory/GUIDE.md") {
		t.Fatalf("user-edited GUIDE must be reported as would-preserve, got:\n%s", report)
	}
	if strings.Contains(report, "would write .codex/mnemon-memory/GUIDE.md") {
		t.Fatalf("user-edited GUIDE must NOT be reported as would-write, got:\n%s", report)
	}
	after, err := os.ReadFile(guide)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "# USER EDIT\n" {
		t.Fatal("dry-run wrote to a user-edited file")
	}
}
