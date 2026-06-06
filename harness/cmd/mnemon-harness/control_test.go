package main

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

// TestControlTokenFileAuth proves P3.2 `control --token-file`: the channel client reads the bearer
// token from a file (so projected hooks keep it out of prompt-visible command lines), authenticates,
// and surfaces explicit errors for a wrong token or a missing file.
func TestControlTokenFileAuth(t *testing.T) {
	root := t.TempDir()
	ref := contract.ResourceRef{Kind: "memory", ID: "m1"}
	rt, err := server.OpenRuntime(filepath.Join(root, server.DefaultStorePath), server.RuntimeConfig{
		Subs:     map[contract.ActorID]contract.Subscription{"codex@project": {Actor: "codex@project", Refs: []contract.ResourceRef{ref}}},
		Bindings: []server.ChannelBinding{server.HostAgentBinding("codex@project", "http://x", []contract.ResourceRef{ref})},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	srv := httptest.NewServer(server.NewRuntimeHandler(rt, server.TokenAuthenticator{Tokens: map[string]contract.ActorID{"tok-codex": "codex@project"}}))
	defer srv.Close()

	tokFile := filepath.Join(t.TempDir(), "codex.token")
	if err := os.WriteFile(tokFile, []byte("tok-codex\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	controlAddr = srv.URL
	controlPrincipal = "codex@project"
	controlToken = ""
	controlTokenFile = tokFile
	controlStatusJSON = false
	t.Cleanup(func() { controlAddr = "http://127.0.0.1:8787"; controlPrincipal = ""; controlToken = ""; controlTokenFile = "" })

	var buf bytes.Buffer
	controlStatusCmd.SetOut(&buf)
	if err := controlStatusCmd.RunE(controlStatusCmd, nil); err != nil {
		t.Fatalf("control status --token-file must succeed: %v", err)
	}
	if !strings.Contains(buf.String(), "codex@project") {
		t.Fatalf("status output must name the token-resolved principal; got %q", buf.String())
	}

	// wrong token => authenticated rejection.
	badTok := filepath.Join(t.TempDir(), "bad.token")
	if err := os.WriteFile(badTok, []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	controlTokenFile = badTok
	if err := controlStatusCmd.RunE(controlStatusCmd, nil); err == nil {
		t.Fatal("control status with an invalid token must fail")
	}

	// missing token file => explicit read error.
	controlTokenFile = filepath.Join(t.TempDir(), "nonexistent.token")
	if err := controlStatusCmd.RunE(controlStatusCmd, nil); err == nil {
		t.Fatal("control status with a missing --token-file must error")
	}
}
