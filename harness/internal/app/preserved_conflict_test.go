package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A file we PRESERVED on conflict (a pre-existing user file at a managed path, or one edited then
// carried through a re-setup) records no ownership hash. A later uninstall must still preserve it —
// not treat the hashless path as generated residue and delete it.
func TestUninstallPreservesPreservedConflict(t *testing.T) {
	// Case 1: pre-existing user file -> survives install AND a later uninstall.
	t.Run("pre-existing survives install then uninstall", func(t *testing.T) {
		root := t.TempDir()
		h := New(root)
		var out bytes.Buffer
		surf := filepath.Join(root, ".codex", "mnemon-memory")
		if err := os.MkdirAll(surf, 0o755); err != nil {
			t.Fatal(err)
		}
		env := filepath.Join(surf, "env.sh")
		if err := os.WriteFile(env, []byte("# USER PRE-EXISTING\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Setup(context.Background(), &out, &out, SetupOptions{
			Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
		}); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := h.SetupUninstall(context.Background(), &out, &out, SetupOptions{
			Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
		}); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		data, err := os.ReadFile(env)
		if err != nil || !bytes.Contains(data, []byte("USER PRE-EXISTING")) {
			t.Fatalf("uninstall deleted a preserved pre-existing file (data=%q err=%v)", data, err)
		}
	})

	// Case 2: a Mnemon file edited by the user, carried through a RE-SETUP (which preserves it as a
	// conflict), must still survive the subsequent uninstall.
	t.Run("edited then re-setup survives uninstall", func(t *testing.T) {
		root := t.TempDir()
		h := New(root)
		var out bytes.Buffer
		opts := SetupOptions{Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root}
		if _, err := h.Setup(context.Background(), &out, &out, opts); err != nil {
			t.Fatalf("setup1: %v", err)
		}
		env := filepath.Join(root, ".codex", "mnemon-memory", "env.sh")
		orig, err := os.ReadFile(env)
		if err != nil {
			t.Fatalf("env not projected: %v", err)
		}
		if err := os.WriteFile(env, append([]byte("# USER EDIT\n"), orig...), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Setup(context.Background(), &out, &out, opts); err != nil { // re-setup preserves the edit
			t.Fatalf("setup2: %v", err)
		}
		if err := h.SetupUninstall(context.Background(), &out, &out, opts); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		data, err := os.ReadFile(env)
		if err != nil || !bytes.Contains(data, []byte("USER EDIT")) {
			t.Fatalf("uninstall deleted a conflict preserved through re-setup (data=%q err=%v)", data, err)
		}
	})
}
