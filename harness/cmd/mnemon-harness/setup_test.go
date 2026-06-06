package main

import (
	"reflect"
	"testing"
)

func TestSetupProductFlagsSelectLoops(t *testing.T) {
	oldLoops := setupLoops
	oldMemory := setupMemory
	oldSkills := setupSkills
	t.Cleanup(func() {
		setupLoops = oldLoops
		setupMemory = oldMemory
		setupSkills = oldSkills
	})

	setupLoops = []string{"memory"}
	setupMemory = true
	setupSkills = true

	got := selectedSetupLoops()
	want := []string{"memory", "skill"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selectedSetupLoops() = %#v, want %#v", got, want)
	}
}
