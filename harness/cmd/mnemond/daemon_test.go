package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// status/down/logs operate on the pidfile + logfile under .mnemon/harness/local without spawning a
// process, so they are unit-testable; the full up→serve→down lifecycle is proven by the e2e leg.

func TestDaemonStatusStoppedWhenNoPidfile(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := daemonStatus([]string{"--root", root}, &out, &out); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "stopped") {
		t.Fatalf("no pidfile must read stopped, got %q", out.String())
	}
}

func TestDaemonStatusRunningForLivePid(t *testing.T) {
	root := t.TempDir()
	dir, pidPath, _ := daemonPaths(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// our own pid is guaranteed alive.
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := daemonStatus([]string{"--root", root}, &out, &out); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "running") {
		t.Fatalf("a live pid must read running, got %q", out.String())
	}
}

func TestDaemonDownStalePidfileIsIdempotent(t *testing.T) {
	root := t.TempDir()
	dir, pidPath, _ := daemonPaths(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// pid 2^30 is not a live process: down must clean the stale pidfile, not error.
	if err := os.WriteFile(pidPath, []byte("1073741824\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := daemonDown([]string{"--root", root}, &out, &out); err != nil {
		t.Fatalf("down on stale pidfile must not error: %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("down must remove the stale pidfile (err=%v)", err)
	}
}

func TestDaemonDownNotRunning(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := daemonDown([]string{"--root", root}, &out, &out); err != nil {
		t.Fatalf("down with no pidfile must be a no-op, got %v", err)
	}
	if !strings.Contains(out.String(), "not running") {
		t.Fatalf("down with no pidfile must report not running, got %q", out.String())
	}
}

func TestDaemonLogsPrintsFile(t *testing.T) {
	root := t.TempDir()
	dir, _, logPath := daemonPaths(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("Local Mnemon: ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := daemonLogs([]string{"--root", root}, &out, &out); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if !strings.Contains(out.String(), "Local Mnemon: ready") {
		t.Fatalf("logs must print the captured output, got %q", out.String())
	}
}

func TestDaemonLogsNoFileYet(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := daemonLogs([]string{"--root", root}, &out, &out); err != nil {
		t.Fatalf("logs with no file must not error: %v", err)
	}
	if !strings.Contains(out.String(), "no log yet") {
		t.Fatalf("logs with no file must say so, got %q", out.String())
	}
}

func TestDaemonPathsUnderLocalStateDir(t *testing.T) {
	_, pidPath, logPath := daemonPaths("/proj")
	if pidPath != filepath.FromSlash("/proj/.mnemon/harness/local/mnemond.pid") {
		t.Fatalf("pidfile path: %s", pidPath)
	}
	if logPath != filepath.FromSlash("/proj/.mnemon/harness/local/mnemond.log") {
		t.Fatalf("logfile path: %s", logPath)
	}
}
