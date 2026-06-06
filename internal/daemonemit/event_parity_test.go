package daemonemit

import "testing"

type eventParityCase struct {
	Name       string
	Topic      string
	Actor      string
	WantAccept bool
}

func TestEventValidationCorpus(t *testing.T) {
	corpus := []eventParityCase{
		{Name: "valid", Topic: "memory.write_candidate_observed", Actor: "host-agent", WantAccept: true},
		{Name: "bad topic", Topic: "memory write", Actor: "host-agent", WantAccept: false},
		{Name: "bad actor", Topic: "memory.write_candidate_observed", Actor: "codex project", WantAccept: false},
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
