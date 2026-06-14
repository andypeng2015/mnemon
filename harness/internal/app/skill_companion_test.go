package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A skill is projected as a single SKILL.md; a user may add companion files (reference.md, scripts) to
// the skill dir. Uninstall must remove only our SKILL.md (and the now-empty dir), never RemoveAll a
// dir that still holds the user's companion files.
func TestUninstallPreservesSkillCompanionFiles(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	var out bytes.Buffer
	opts := SetupOptions{Host: "codex", Loops: []string{"skill"}, Principal: "codex@project", ProjectRoot: root}
	if _, err := h.Setup(context.Background(), &out, &out, opts); err != nil {
		t.Fatalf("setup: %v", err)
	}

	skillDir := filepath.Join(root, ".codex", "skills", "skill-observe")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("skill not projected: %v", err)
	}
	companion := filepath.Join(skillDir, "reference.md")
	if err := os.WriteFile(companion, []byte("# user companion notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := h.SetupUninstall(context.Background(), &out, &out, opts); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	if _, err := os.Stat(companion); err != nil {
		t.Fatalf("uninstall deleted a user companion file in the skill dir: %v", err)
	}
}
