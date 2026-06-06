package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// parityCase mirrors the shared event-validation corpus that pins the release-side
// internal/daemonemit writer's rule-set to this package's ValidateEvent.
//
// schema is the canonical validator (eventlog.Append validates through
// ValidateEvent); daemonemit is a SECOND writer to the same .mnemon/events.jsonl
// that enforces its own copy of the allowed-actor list + event-type regex. To stop
// the two rule-sets from drifting we assert both against ONE corpus from two sides.
// We do NOT import across the trees: a harness->release (or release->harness) import
// would breach the decoupling (D5, "zero imports either way"). The corpus lives next
// to this canonical validator; daemonemit reads the same file from its own test.
type parityCase struct {
	Name       string `json:"name"`
	Topic      string `json:"topic"`
	Actor      string `json:"actor"`
	WantAccept bool   `json:"want_accept"`
}

func loadEventParityCorpus(t *testing.T, path string) []parityCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read parity corpus: %v", err)
	}
	var cases []parityCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode parity corpus: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("parity corpus is empty")
	}
	return cases
}

// TestEventValidationCorpusParity asserts this package's ValidateEvent accepts/rejects
// each corpus case as the corpus declares. internal/daemonemit asserts the SAME corpus
// against its NewEvent; agreement on both sides == the two writers share one rule-set.
func TestEventValidationCorpusParity(t *testing.T) {
	corpus := loadEventParityCorpus(t, filepath.Join("testdata", "event_validation_corpus.json"))
	for _, c := range corpus {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			event := Event{
				SchemaVersion: Version,
				ID:            "evt_parity",
				TS:            "2026-06-06T12:00:00Z",
				Type:          c.Topic,
				Actor:         c.Actor,
				Source:        "test.parity",
				CorrelationID: "parity:1",
				Payload:       map[string]any{},
			}
			gotAccept := ValidateEvent(event) == nil
			if gotAccept != c.WantAccept {
				t.Fatalf("schema.ValidateEvent accept=%v, want %v (topic=%q actor=%q)", gotAccept, c.WantAccept, c.Topic, c.Actor)
			}
		})
	}
}
