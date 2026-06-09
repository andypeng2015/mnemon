package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Uninstall must not delete a projected skill the user has hand-edited: only skills still ours (hash
// matches what we recorded) are removed; a user-modified one is preserved.
func TestUninstallPreservesUserEditedSkill(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	var out bytes.Buffer
	if _, err := h.Setup(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	skill := filepath.Join(root, ".codex", "skills", "memory-get", "SKILL.md")
	orig, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("projected skill missing: %v", err)
	}
	if err := os.WriteFile(skill, append([]byte("# USER EDIT — keep me\n\n"), orig...), 0o644); err != nil {
		t.Fatalf("edit skill: %v", err)
	}

	if err := h.SetupUninstall(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("uninstall removed a user-edited skill: %v", err)
	}
	if !bytes.Contains(after, []byte("USER EDIT")) {
		t.Fatal("uninstall clobbered the user's skill edit")
	}
}

// Uninstall must apply the ownership-hash no-clobber to ALL managed files, not just skills: a
// user-edited projected hook and GUIDE must survive an uninstall.
func TestUninstallPreservesUserEditedHookAndGuide(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	var out bytes.Buffer
	if _, err := h.Setup(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	guide := filepath.Join(root, ".codex", "mnemon-memory", "GUIDE.md")
	hook := filepath.Join(root, ".codex", "hooks", "mnemon-memory", "prime.sh")
	for _, f := range []string{guide, hook} {
		orig, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("projected file missing %s: %v", f, err)
		}
		if err := os.WriteFile(f, append([]byte("# USER EDIT — keep me\n"), orig...), 0o644); err != nil {
			t.Fatalf("edit %s: %v", f, err)
		}
	}

	if err := h.SetupUninstall(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	for _, f := range []string{guide, hook} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("uninstall removed a user-edited managed file %s: %v", f, err)
		}
		if !bytes.Contains(data, []byte("USER EDIT")) {
			t.Fatalf("uninstall clobbered the user edit in %s", f)
		}
	}
}
