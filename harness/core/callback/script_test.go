package callback

import (
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

// A script callback receives the event JSON on stdin and emits a JSON array of proposed events on stdout.
func TestScriptCallbackParsesStdoutEmitsIntent(t *testing.T) {
	reg := NewRegistry()
	reg.On("x.observed", NewScriptCallback("sh", 5*time.Second, "-c",
		`cat >/dev/null; printf '[{"Type":"y.proposed","Payload":{"k":"v"}}]'`))
	intents := reg.Dispatch(contract.Event{Type: "x.observed"}, projection.Projection{})
	if len(intents) != 1 || intents[0].Type != "y.proposed" {
		t.Fatalf("script must emit one parsed intent, got %+v", intents)
	}
}

// Garbage stdout parses to nothing -> the callback errors -> Dispatch drops ALL its intents (not committed).
func TestScriptCallbackGarbageStdoutYieldsZeroIntents(t *testing.T) {
	reg := NewRegistry()
	reg.On("x.observed", NewScriptCallback("sh", 5*time.Second, "-c",
		`cat >/dev/null; printf 'not json at all'`))
	if n := len(reg.Dispatch(contract.Event{Type: "x.observed"}, projection.Projection{})); n != 0 {
		t.Fatalf("garbage stdout must yield zero intents (not committed), got %d", n)
	}
}

// A script that exits non-zero (or times out) also contributes zero intents.
func TestScriptCallbackNonZeroExitYieldsZeroIntents(t *testing.T) {
	reg := NewRegistry()
	reg.On("x.observed", NewScriptCallback("sh", 5*time.Second, "-c", `exit 3`))
	if n := len(reg.Dispatch(contract.Event{Type: "x.observed"}, projection.Projection{})); n != 0 {
		t.Fatalf("failed script must yield zero intents, got %d", n)
	}
}
