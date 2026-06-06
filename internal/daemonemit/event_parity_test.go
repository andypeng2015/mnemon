package daemonemit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// eventParityCase mirrors the shared corpus owned by the canonical validator at
// harness/internal/lifecycle/schema. daemonemit is a SECOND writer to the same
// .mnemon/events.jsonl with its own copy of the allowed-actor list + event-type
// regex; this test pins that copy to schema.ValidateEvent's behaviour by asserting
// NewEvent's accept/reject against the SAME corpus the schema-side test asserts.
//
// We read the corpus as a file rather than importing the schema package: a
// release->harness import would breach the RELEASE<->harness decoupling (D5,
// "zero imports either way"). A file read crosses no import edge.
type eventParityCase struct {
	Name       string `json:"name"`
	Topic      string `json:"topic"`
	Actor      string `json:"actor"`
	WantAccept bool   `json:"want_accept"`
}

func TestEventValidationCorpusParity(t *testing.T) {
	corpusPath := filepath.Join("..", "..", "harness", "internal", "lifecycle", "schema", "testdata", "event_validation_corpus.json")
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatalf("read shared parity corpus %s: %v", corpusPath, err)
	}
	var corpus []eventParityCase
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("decode shared parity corpus: %v", err)
	}
	if len(corpus) == 0 {
		t.Fatal("parity corpus is empty")
	}
	for _, c := range corpus {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			_, err := NewEvent(Options{Topic: c.Topic, Actor: c.Actor})
			gotAccept := err == nil
			if gotAccept != c.WantAccept {
				t.Fatalf("daemonemit.NewEvent accept=%v, want %v (topic=%q actor=%q)", gotAccept, c.WantAccept, c.Topic, c.Actor)
			}
		})
	}
}
