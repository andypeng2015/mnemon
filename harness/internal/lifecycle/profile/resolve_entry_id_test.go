package profile

import (
	"testing"
	"time"
)

// TestResolveEntryIDIsCleanFixedPoint pins the P2 re-verification fix: ResolveEntryID must return
// a cleanID FIXED POINT so a governed caller can feed the SAME id to both the kernel write and
// AddEntry without divergence. The generated-id branch embeds an uppercase-T timestamp that
// AddEntry would lower-case on re-clean — the kernel id would then not be findable in the host
// file. cleanID(ResolveEntryID(...)) == ResolveEntryID(...) closes that.
func TestResolveEntryIDIsCleanFixedPoint(t *testing.T) {
	now := time.Date(2026, 6, 6, 6, 30, 54, 0, time.UTC)
	cases := []struct{ entryID, typ, summary string }{
		{"", "fact", "likes tea"},       // generated-id branch (the regression)
		{"Already Clean?", "note", "s"}, // non-canonical explicit id
		{"pref-1", "preference", "s"},   // already canonical
		{"   ", "fact", "spaced"},       // whitespace -> generated
	}
	for _, c := range cases {
		id := ResolveEntryID(c.entryID, c.typ, c.summary, now)
		if id == "" {
			t.Fatalf("ResolveEntryID(%q,...) must be non-empty", c.entryID)
		}
		if cleanID(id) != id {
			t.Fatalf("ResolveEntryID(%q,...) = %q is not a cleanID fixed point (cleanID=%q)", c.entryID, id, cleanID(id))
		}
	}
}
