package hostsurface

import (
	"os"
	"path/filepath"
	"testing"
)

// classifyManaged is the 3-state no-clobber decision for a managed definition file: write when the
// file is absent or still matches what we last wrote; preserve (conflict) when the user has edited a
// previously-managed file; and, with no prior record, adopt on install but never on a refresh.
func TestClassifyManaged(t *testing.T) {
	dir := t.TempDir()
	desired := []byte("desired content\n")

	t.Run("absent file writes", func(t *testing.T) {
		if got := classifyManaged(filepath.Join(dir, "absent"), desired, "", false); got != classWrite {
			t.Fatalf("absent file must write; got %v", got)
		}
	})

	t.Run("prior-match writes", func(t *testing.T) {
		dst := filepath.Join(dir, "ours")
		mustWrite(t, dst, desired)
		if got := classifyManaged(dst, []byte("an update"), hashBytes(desired), false); got != classWrite {
			t.Fatalf("a file unmodified since we wrote it must write; got %v", got)
		}
	})

	t.Run("user-modified conflicts", func(t *testing.T) {
		dst := filepath.Join(dir, "edited")
		mustWrite(t, dst, []byte("the user changed this"))
		if got := classifyManaged(dst, desired, hashBytes([]byte("what we last wrote")), false); got != classConflict {
			t.Fatalf("a user-edited managed file must be preserved; got %v", got)
		}
	})

	t.Run("nil-prior differing file: install adopts, refresh preserves", func(t *testing.T) {
		dst := filepath.Join(dir, "preexisting")
		mustWrite(t, dst, []byte("pre-existing unmanaged content"))
		if got := classifyManaged(dst, desired, "", true); got != classConflict {
			t.Fatalf("refresh must not adopt an unknown differing file; got %v", got)
		}
		if got := classifyManaged(dst, desired, "", false); got != classWrite {
			t.Fatalf("install must adopt an unknown differing file; got %v", got)
		}
	})
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
