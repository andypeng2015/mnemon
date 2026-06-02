package callback

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"

	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/projection"
)

// ScriptCallback is a TRUSTED-AUTHOR subprocess callback: it pipes the event JSON to a child process on
// stdin and parses the child's stdout as a JSON array of ProposedEvents. A non-zero exit, a timeout, or
// unparseable stdout yields an error -> Dispatch drops ALL of its intents (nothing is committed).
//
// HONESTY NOTE (Invariant #14/#15): plain os/exec is NOT an in-process sandbox — the child inherits this
// process's env/cwd/fs/net and could, e.g., `sqlite3 <coreplane.db> "UPDATE ..."` directly, bypassing the
// kernel. ScriptCallback is therefore an extension point for TRUSTED authors only. Invariant #15's
// UNTRUSTED control agent MUST be an in-process BuiltinFunc (no FS reach), never a script. Operationally:
// keep coreplane.db mode-0600 so a stray script cannot open it.
type ScriptCallback struct {
	Path    string
	Args    []string
	Timeout time.Duration
}

func NewScriptCallback(path string, timeout time.Duration, args ...string) *ScriptCallback {
	return &ScriptCallback{Path: path, Args: args, Timeout: timeout}
}

func (s *ScriptCallback) OnEvent(ev contract.Event, _ projection.Projection) ([]contract.ProposedEvent, error) {
	in, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Stdin = bytes.NewReader(in)
	out, err := cmd.Output() // non-zero exit / timeout -> err -> caller (Dispatch) drops intents
	if err != nil {
		return nil, err
	}
	var proposed []contract.ProposedEvent
	if err := json.Unmarshal(bytes.TrimSpace(out), &proposed); err != nil {
		return nil, err // garbage stdout -> error -> zero intents (never committed)
	}
	return proposed, nil
}
