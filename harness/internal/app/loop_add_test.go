package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
)

const widgetPackageSpec = `{"schema_version":1,"name":"widget","observed_type":"widget.write_candidate.observed",
"proposed_type":"widget.write.proposed","resource_kind":"widget","items_field":"items",
"fields":[{"name":"text","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# Widgets","field":"text"}}}}`

// loop add places a package under its canonical name and validates it through the boot resolution;
// the registered package then resolves in the project catalog.
func TestLoopAddRegistersAndValidates(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "widget")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "capability.json"), []byte(widgetPackageSpec), 0o644); err != nil {
		t.Fatal(err)
	}

	name, err := New(root).LoopAdd(src)
	if err != nil {
		t.Fatalf("loop add: %v", err)
	}
	if name != "widget" {
		t.Fatalf("registered name = %q, want widget", name)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "loops", "widget", "capability.json")); err != nil {
		t.Fatalf("package not placed under .mnemon/loops/widget: %v", err)
	}
	catalog, err := capability.ResolveCatalog(root, kernel.DefaultSchemaGuard().Required)
	if err != nil {
		t.Fatalf("resolve after add: %v", err)
	}
	if _, ok := catalog["widget"]; !ok {
		t.Fatalf("added loop must resolve in the catalog: %v", catalog)
	}
}

// A package that would refuse boot is rejected AND rolled back — no half-added directory lingers.
func TestLoopAddRejectsAndRollsBack(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "broken")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	// resource_kind "memory" is a first-party kind an external package may not claim (shadowing) —
	// ResolveCatalog refuses it, so loop add must too.
	bad := `{"schema_version":1,"name":"broken","observed_type":"broken.write_candidate.observed",
"proposed_type":"broken.write.proposed","resource_kind":"memory","items_field":"items",
"fields":[{"name":"text","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# B","field":"text"}}}}`
	if err := os.WriteFile(filepath.Join(src, "capability.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).LoopAdd(src); err == nil {
		t.Fatal("loop add must reject a package that fails boot resolution")
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "loops", "broken")); !os.IsNotExist(err) {
		t.Fatalf("a rejected package must be rolled back, but .mnemon/loops/broken survives (err=%v)", err)
	}
}

// An existing target is not overwritten — the user removes it first to replace.
func TestLoopAddRefusesExistingTarget(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "widget")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "capability.json"), []byte(widgetPackageSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).LoopAdd(src); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, err := New(root).LoopAdd(src); err == nil {
		t.Fatal("a second add of an existing target must refuse, not overwrite")
	}
}
