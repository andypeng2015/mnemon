package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	hruntime "github.com/mnemon-dev/mnemon/harness/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	codexTeamAddr        string
	codexTeamControlAddr string
	codexTeamStorePath   string
	codexTeamAgents      int
	codexTeamInterval    time.Duration
	codexTeamTaskTimeout time.Duration
	codexTeamTurnTimeout time.Duration
	codexTeamTasks       []string
	codexTeamBackend     string
	codexTeamRounds      int
	codexTeamCodexCmd    string
	codexTeamSandbox     string
)

var codexTeamCmd = &cobra.Command{
	Use:   "codex-team",
	Short: "Run a local multi-Codex appserver demo with a live Web UI",
	Long: "Run an experimental local demo: a Local Mnemon runtime, multiple Codex appserver " +
		"workers executing local tasks, writing governed observations through the channel, and a " +
		"browser UI showing task flow plus GOAL/FIELD/INBOX/LEDGER activity.",
	RunE: runCodexTeam,
}

func init() {
	codexTeamCmd.Flags().StringVar(&codexTeamAddr, "addr", "127.0.0.1:8795", "Web UI listen address")
	codexTeamCmd.Flags().StringVar(&codexTeamControlAddr, "control-addr", "127.0.0.1:0", "internal Local Mnemon channel listen address")
	codexTeamCmd.Flags().StringVar(&codexTeamStorePath, "store", "", "governed.db path (default: temp demo store)")
	codexTeamCmd.Flags().IntVar(&codexTeamAgents, "agents", 5, "number of local Codex appserver workers")
	codexTeamCmd.Flags().DurationVar(&codexTeamInterval, "interval", 2500*time.Millisecond, "appserver observation cadence")
	codexTeamCmd.Flags().DurationVar(&codexTeamTaskTimeout, "task-timeout", 5*time.Minute, "timeout for each local task command")
	codexTeamCmd.Flags().DurationVar(&codexTeamTurnTimeout, "turn-timeout", 10*time.Minute, "timeout for each real Codex app-server turn")
	codexTeamCmd.Flags().StringArrayVar(&codexTeamTasks, "task", nil, "task as id=prompt/command; may be repeated (default: five collaborative lanes)")
	codexTeamCmd.Flags().StringVar(&codexTeamBackend, "backend", "codex", "worker backend: codex or shell")
	codexTeamCmd.Flags().IntVar(&codexTeamRounds, "rounds", 4, "rounds per real Codex appserver task")
	codexTeamCmd.Flags().StringVar(&codexTeamCodexCmd, "codex-command", "codex", "Codex CLI command used to start real app-servers")
	codexTeamCmd.Flags().StringVar(&codexTeamSandbox, "codex-sandbox", "readOnly", "Codex turn sandbox policy type: readOnly, workspaceWrite, or dangerFullAccess")
	codexTeamCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(codexTeamCmd)
}

func runCodexTeam(cmd *cobra.Command, args []string) error {
	if codexTeamAgents < 1 || codexTeamAgents > 12 {
		return fmt.Errorf("--agents must be between 1 and 12")
	}
	if codexTeamInterval < 500*time.Millisecond {
		return fmt.Errorf("--interval must be at least 500ms")
	}
	if codexTeamTaskTimeout <= 0 {
		return fmt.Errorf("--task-timeout must be positive")
	}
	if codexTeamTurnTimeout <= 0 {
		return fmt.Errorf("--turn-timeout must be positive")
	}
	if codexTeamRounds < 1 {
		return fmt.Errorf("--rounds must be at least 1")
	}
	if codexTeamBackend != "codex" && codexTeamBackend != "shell" {
		return fmt.Errorf("--backend must be codex or shell")
	}
	tasks, err := codexTeamTaskSpecs(codexTeamTasks, codexTeamBackend)
	if err != nil {
		return err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	tmpDir := ""
	storePath := codexTeamStorePath
	if storePath == "" {
		var err error
		tmpDir, err = os.MkdirTemp("", "mnemon-codex-team-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		storePath = filepath.Join(tmpDir, "governed.db")
	}
	dynamicRoot := ""
	if tmpDir != "" {
		dynamicRoot = filepath.Join(tmpDir, "dynamic-root")
	} else {
		var err error
		dynamicRoot, err = os.MkdirTemp("", "mnemon-codex-team-dynamic-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dynamicRoot)
	}

	controlLn, err := net.Listen("tcp", codexTeamControlAddr)
	if err != nil {
		return fmt.Errorf("listen control channel: %w", err)
	}
	controlURL := listenerURL(controlLn)
	bindings, tokens, err := codexTeamBindings(codexTeamAgents, controlURL)
	if err != nil {
		_ = controlLn.Close()
		return err
	}
	tokenFiles, cleanupTokenFiles, err := codexTeamWriteTokenFiles(tokens)
	if err != nil {
		_ = controlLn.Close()
		return err
	}
	defer cleanupTokenFiles()
	harnessBinary, err := os.Executable()
	if err != nil {
		_ = controlLn.Close()
		return err
	}
	runtimeHandle, err := newCodexTeamRuntimeHandle(storePath, dynamicRoot, bindings, tokens)
	if err != nil {
		_ = controlLn.Close()
		return err
	}
	defer runtimeHandle.Close()

	controlSrv := &http.Server{Handler: runtimeHandle}
	uiLn, err := net.Listen("tcp", codexTeamAddr)
	if err != nil {
		_ = controlLn.Close()
		return fmt.Errorf("listen Web UI: %w", err)
	}
	uiURL := listenerURL(uiLn)

	state := newCodexTeamState(bindings, tasks, codexTeamRounds)
	viewText := func(principal contract.ActorID, task codexTaskStatus) string {
		view, err := runtimeHandle.BuildTowerView()
		if err != nil {
			return "Tower unavailable: " + err.Error()
		}
		return codexTeamTowerBrief(view, principal, task, state.protocolSnapshot())
	}
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for i, b := range bindings {
		if b.ActorKind != contract.KindHostAgent {
			continue
		}
		token := codexTeamTokenForPrincipal(tokens, b.Principal)
		wg.Add(1)
		go func(index int, binding channel.ChannelBinding, tok string) {
			defer wg.Done()
			if codexTeamBackend == "codex" {
				runRealCodexAppserver(workerCtx, index, binding.Principal, tok, tokenFiles[binding.Principal], harnessBinary, controlURL, codexTeamInterval, codexTeamTurnTimeout, workDir, state, viewText)
				return
			}
			runShellCodexAppserver(workerCtx, index, binding.Principal, tok, controlURL, codexTeamInterval, codexTeamTaskTimeout, workDir, state)
		}(i, b, token)
	}
	go runCodexTeamProtocolEvolution(workerCtx, runtimeHandle, state, controlURL, codexTeamTokenForPrincipal(tokens, "human@owner"))

	uiSrv := &http.Server{Handler: codexTeamMux(runtimeHandle, state, codexTeamSnapshotMeta{
		ControlURL:  controlURL,
		StorePath:   storePath,
		DynamicRoot: dynamicRoot,
		StartedAt:   state.startedAt,
	})}

	errc := make(chan error, 2)
	go func() {
		if err := controlSrv.Serve(controlLn); err != nil && err != http.ErrServerClosed {
			errc <- fmt.Errorf("control channel stopped: %w", err)
		}
	}()
	go func() {
		if err := uiSrv.Serve(uiLn); err != nil && err != http.ErrServerClosed {
			errc <- fmt.Errorf("Web UI stopped: %w", err)
		}
	}()

	fmt.Fprintf(cmd.OutOrStdout(), "Codex Team Web UI: %s\n", uiURL)
	fmt.Fprintf(cmd.OutOrStdout(), "Local Mnemon channel: %s\n", controlURL)
	fmt.Fprintf(cmd.OutOrStdout(), "Backend: %s\n", codexTeamBackend)
	fmt.Fprintf(cmd.OutOrStdout(), "Appservers: %d Codex principals\n", codexTeamAgents)
	fmt.Fprintf(cmd.OutOrStdout(), "Tasks: %d collaborative lanes\n", len(tasks))
	fmt.Fprintf(cmd.OutOrStdout(), "Store: %s\n", storePath)
	fmt.Fprintf(cmd.OutOrStdout(), "Dynamic protocol root: %s\n", dynamicRoot)

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}

	cancelWorkers()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdown()
	_ = uiSrv.Shutdown(shutdownCtx)
	_ = controlSrv.Shutdown(shutdownCtx)
	wg.Wait()
	return runErr
}

type codexTeamSnapshotMeta struct {
	ControlURL  string
	StorePath   string
	DynamicRoot string
	StartedAt   time.Time
}

type codexTeamRuntimeHandle struct {
	mu          sync.RWMutex
	storePath   string
	projectRoot string
	bindings    []channel.ChannelBinding
	auth        channel.TokenAuthenticator
	rt          *hruntime.Runtime
	handler     http.Handler
	catalog     map[string]capability.Capability
}

func newCodexTeamRuntimeHandle(storePath, projectRoot string, bindings []channel.ChannelBinding, tokens map[string]contract.ActorID) (*codexTeamRuntimeHandle, error) {
	h := &codexTeamRuntimeHandle{
		storePath:   storePath,
		projectRoot: projectRoot,
		bindings:    append([]channel.ChannelBinding(nil), bindings...),
		auth:        channel.TokenAuthenticator{Tokens: tokens},
	}
	if err := h.open(nil); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *codexTeamRuntimeHandle) open(catalog map[string]capability.Capability) error {
	rc, err := app.LocalRuntimeConfigFromBindings(h.bindings, catalog)
	if err != nil {
		return fmt.Errorf("assemble local runtime: %w", err)
	}
	rt, err := hruntime.OpenRuntime(h.storePath, rc)
	if err != nil {
		return fmt.Errorf("open runtime: %w", err)
	}
	h.rt = rt
	h.handler = hruntime.NewRuntimeHandler(rt, h.auth)
	if catalog == nil {
		catalog = capability.EmbeddedCatalog()
	}
	h.catalog = catalog
	return nil
}

func (h *codexTeamRuntimeHandle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	handler := h.handler
	if handler == nil {
		http.Error(w, "runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

func (h *codexTeamRuntimeHandle) BuildTowerView() (app.TowerView, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return app.TowerView{}, fmt.Errorf("runtime unavailable")
	}
	return app.BuildTowerView(h.rt, h.bindings)
}

func (h *codexTeamRuntimeHandle) RuntimeEvents() []codexObservedEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return nil
	}
	return codexTeamRuntimeEvents(h.rt)
}

func (h *codexTeamRuntimeHandle) MaterializeLoopdefs() ([]string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	return codexTeamMaterializeLoopdefs(h.rt, h.projectRoot)
}

func (h *codexTeamRuntimeHandle) ReloadFromDynamicRoot() error {
	catalog, err := capability.ResolveCatalog(h.projectRoot, kernel.DefaultSchemaGuard().Required)
	if err != nil {
		return fmt.Errorf("resolve dynamic catalog: %w", err)
	}
	rc, err := app.LocalRuntimeConfigFromBindings(h.bindings, catalog)
	if err != nil {
		return fmt.Errorf("assemble reloaded runtime: %w", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rt != nil {
		_ = h.rt.Close()
	}
	rt, err := hruntime.OpenRuntime(h.storePath, rc)
	if err != nil {
		h.rt = nil
		h.handler = nil
		return fmt.Errorf("open reloaded runtime: %w", err)
	}
	h.rt = rt
	h.handler = hruntime.NewRuntimeHandler(rt, h.auth)
	h.catalog = catalog
	return nil
}

func (h *codexTeamRuntimeHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rt == nil {
		return nil
	}
	err := h.rt.Close()
	h.rt = nil
	h.handler = nil
	return err
}

type codexTeamSnapshot struct {
	Now         string                 `json:"now"`
	Uptime      string                 `json:"uptime"`
	ControlURL  string                 `json:"control_url"`
	StorePath   string                 `json:"store_path"`
	DynamicRoot string                 `json:"dynamic_root"`
	Appservers  []codexAppserverStatus `json:"appservers"`
	Tasks       []codexTaskStatus      `json:"tasks"`
	Messages    []codexTeamMessage     `json:"messages"`
	Events      []codexObservedEvent   `json:"events"`
	Protocols   []codexProtocolStatus  `json:"protocols"`
	Tower       app.TowerView          `json:"tower"`
	Counts      codexTeamCounts        `json:"counts"`
}

type codexTeamCounts struct {
	Agents      int `json:"agents"`
	Assignments int `json:"assignments"`
	Tasks       int `json:"tasks"`
	Running     int `json:"running"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Inbox       int `json:"inbox"`
	Ledger      int `json:"ledger"`
	Progress    int `json:"progress"`
	Protocols   int `json:"protocols"`
}

func codexTeamMux(runtimeHandle *codexTeamRuntimeHandle, state *codexTeamState, meta codexTeamSnapshotMeta) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = codexTeamHTML.Execute(w, nil)
	})
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		view, err := runtimeHandle.BuildTowerView()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events := state.eventSnapshot()
		if runtimeEvents := runtimeHandle.RuntimeEvents(); runtimeEvents != nil {
			events = runtimeEvents
		}
		state.mergeProtocolUsage(events)
		protocols := state.protocolSnapshot()
		appservers := state.snapshot()
		snap := codexTeamSnapshot{
			Now:         time.Now().UTC().Format(time.RFC3339),
			Uptime:      time.Since(meta.StartedAt).Round(time.Second).String(),
			ControlURL:  meta.ControlURL,
			StorePath:   meta.StorePath,
			DynamicRoot: meta.DynamicRoot,
			Appservers:  appservers,
			Tasks:       state.taskSnapshot(),
			Messages:    state.messageSnapshot(),
			Events:      events,
			Protocols:   protocols,
			Tower:       view,
			Counts: codexTeamCounts{
				Agents:      len(appservers),
				Assignments: len(view.Field.Assignments),
				Tasks:       state.taskCount(),
				Running:     state.taskStateCount("running"),
				Passed:      state.taskStateCount("passed"),
				Failed:      state.taskStateCount("failed"),
				Inbox:       len(view.Inbox.Escalations),
				Ledger:      len(view.Ledger.Decisions),
				Progress:    len(view.Goal.Progress),
				Protocols:   len(protocols),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	return mux
}

func codexTeamRuntimeEvents(rt *hruntime.Runtime) []codexObservedEvent {
	events, err := rt.PendingEvents(0)
	if err != nil {
		return nil
	}
	out := make([]codexObservedEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, codexObservedEvent{
			At:        ev.TS,
			Seq:       ev.IngestSeq,
			Principal: string(ev.Actor),
			Type:      ev.Type,
			Summary:   codexTeamEventSummary(ev),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	if len(out) > 200 {
		out = append([]codexObservedEvent(nil), out[len(out)-200:]...)
	}
	return out
}

func codexTeamEventSummary(ev contract.Event) string {
	for _, key := range []string{"summary", "statement", "scope", "content", "reason", "claim", "decision", "next_action"} {
		if s, ok := ev.Payload[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	if poc, ok := ev.Payload["poc"].(map[string]any); ok {
		if claim, ok := poc["claim"].(string); ok && strings.TrimSpace(claim) != "" {
			return claim
		}
	}
	return ev.Type
}

type codexTeamState struct {
	mu        sync.Mutex
	startedAt time.Time
	runID     string
	servers   map[contract.ActorID]codexAppserverStatus
	tasks     []codexTaskStatus
	events    []codexObservedEvent
	messages  []codexTeamMessage
	protocols map[string]codexProtocolStatus
}

type codexAppserverStatus struct {
	ID            string `json:"id"`
	Principal     string `json:"principal"`
	Role          string `json:"role"`
	State         string `json:"state"`
	Observations  int    `json:"observations"`
	LastSeq       int64  `json:"last_seq"`
	LastEventType string `json:"last_event_type"`
	LastSummary   string `json:"last_summary"`
	LastError     string `json:"last_error"`
	UpdatedAt     string `json:"updated_at"`
}

type codexTaskSpec struct {
	ID      string
	Title   string
	Command string
}

type codexTaskStatus struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Command    string `json:"command"`
	State      string `json:"state"`
	Assignee   string `json:"assignee"`
	ThreadID   string `json:"thread_id"`
	Round      int    `json:"round"`
	Rounds     int    `json:"rounds"`
	Attempts   int    `json:"attempts"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Duration   string `json:"duration"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Error      string `json:"error"`
}

type codexObservedEvent struct {
	At        string `json:"at"`
	Seq       int64  `json:"seq"`
	Principal string `json:"principal"`
	Type      string `json:"type"`
	Summary   string `json:"summary"`
	Error     string `json:"error"`
}

type codexProtocolStatus struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	Source       string `json:"source"`
	ObservedType string `json:"observed_type"`
	ProposedType string `json:"proposed_type"`
	Resource     string `json:"resource"`
	Purpose      string `json:"purpose"`
	Summary      string `json:"summary"`
	UpdatedAt    string `json:"updated_at"`
	Uses         int    `json:"uses"`
}

type codexTeamMessage struct {
	At        string `json:"at"`
	Principal string `json:"principal"`
	TaskID    string `json:"task_id"`
	Round     int    `json:"round"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
}

type codexTaskResult struct {
	Duration time.Duration
	ExitCode int
	Output   string
	Err      error
}

func newCodexTeamState(bindings []channel.ChannelBinding, tasks []codexTaskSpec, rounds int) *codexTeamState {
	now := time.Now()
	s := &codexTeamState{
		startedAt: now,
		runID:     fmt.Sprintf("%d", now.UnixNano()),
		servers:   map[contract.ActorID]codexAppserverStatus{},
		tasks:     make([]codexTaskStatus, 0, len(tasks)),
		protocols: map[string]codexProtocolStatus{},
	}
	s.protocols["progress_digest"] = codexProtocolStatus{
		Name:         "progress_digest",
		Status:       "active",
		Source:       "builtin",
		ObservedType: "progress_digest.write_candidate.observed",
		ProposedType: "progress_digest.write.proposed",
		Resource:     "progress_digest/project",
		Purpose:      "coarse lane progress before richer collaboration semantics exist",
		Summary:      "built-in coordination event family",
		UpdatedAt:    now.UTC().Format(time.RFC3339),
	}
	s.protocols["loopdef"] = codexProtocolStatus{
		Name:         "loopdef",
		Status:       "active",
		Source:       "builtin",
		ObservedType: "loopdef.write_candidate.observed",
		ProposedType: "loopdef.write.proposed",
		Resource:     "loopdef/project",
		Purpose:      "governed runtime definition of new event families",
		Summary:      "operator-gated protocol evolution path",
		UpdatedAt:    now.UTC().Format(time.RFC3339),
	}
	for _, p := range codexTeamDynamicProtocols() {
		s.protocols[p.Name] = p
	}
	for _, b := range bindings {
		if b.ActorKind != contract.KindHostAgent {
			continue
		}
		s.servers[b.Principal] = codexAppserverStatus{
			ID:        strings.TrimSuffix(string(b.Principal), "@appserver"),
			Principal: string(b.Principal),
			Role:      codexTeamRole(string(b.Principal)),
			State:     "booting",
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}
	for _, t := range tasks {
		s.tasks = append(s.tasks, codexTaskStatus{
			ID:       t.ID,
			Title:    t.Title,
			Command:  t.Command,
			State:    "pending",
			Rounds:   rounds,
			ExitCode: -1,
		})
	}
	return s
}

func (s *codexTeamState) record(principal contract.ActorID, eventType, summary string, seq int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	st, knownAgent := s.servers[principal]
	st.UpdatedAt = now
	errText := ""
	if err != nil {
		errText = err.Error()
		if knownAgent {
			st.State = "error"
			st.LastError = errText
			st.LastSummary = summary
			s.servers[principal] = st
		}
		s.appendEventLocked(codexObservedEvent{At: now, Seq: seq, Principal: string(principal), Type: eventType, Summary: summary, Error: errText})
		return
	}
	if knownAgent {
		if st.State == "" || st.State == "booting" {
			st.State = "observing"
		}
		st.Observations++
		st.LastSeq = seq
		st.LastEventType = eventType
		st.LastSummary = summary
		st.LastError = ""
		s.servers[principal] = st
	}
	s.markProtocolUsedLocked(eventType, summary, now)
	s.appendEventLocked(codexObservedEvent{At: now, Seq: seq, Principal: string(principal), Type: eventType, Summary: summary})
}

func (s *codexTeamState) setAgentState(principal contract.ActorID, stateName, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.servers[principal]
	st.State = stateName
	st.LastSummary = summary
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.servers[principal] = st
}

func (s *codexTeamState) appendEventLocked(ev codexObservedEvent) {
	s.events = append(s.events, ev)
	if len(s.events) > 200 {
		s.events = append([]codexObservedEvent(nil), s.events[len(s.events)-200:]...)
	}
}

func (s *codexTeamState) appendMessage(principal contract.ActorID, taskID string, round int, kind, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := codexTeamMessage{
		At:        time.Now().UTC().Format(time.RFC3339),
		Principal: string(principal),
		TaskID:    taskID,
		Round:     round,
		Kind:      kind,
		Text:      codexTeamTrimOutput(text, 2500),
	}
	s.messages = append(s.messages, msg)
	if len(s.messages) > 120 {
		s.messages = append([]codexTeamMessage(nil), s.messages[len(s.messages)-120:]...)
	}
}

func (s *codexTeamState) snapshot() []codexAppserverStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]codexAppserverStatus, 0, len(s.servers))
	for _, st := range s.servers {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Principal < out[j].Principal })
	return out
}

func (s *codexTeamState) startTaskRound(principal contract.ActorID, taskID, threadID string, round int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range s.tasks {
		if s.tasks[i].ID != taskID {
			continue
		}
		s.tasks[i].Round = round
		s.tasks[i].ThreadID = threadID
		s.tasks[i].Duration = time.Since(parseCodexTeamTime(s.tasks[i].StartedAt)).Round(time.Second).String()
		break
	}
	st := s.servers[principal]
	st.State = "running"
	st.LastSummary = fmt.Sprintf("running %s round %d", taskID, round)
	st.UpdatedAt = now
	s.servers[principal] = st
}

func (s *codexTeamState) claimTask(principal contract.ActorID) (codexTaskStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		if s.tasks[i].State != "pending" {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		s.tasks[i].State = "running"
		s.tasks[i].Assignee = string(principal)
		s.tasks[i].StartedAt = now
		s.tasks[i].Attempts++
		st := s.servers[principal]
		st.State = "running"
		st.LastSummary = "running " + s.tasks[i].ID
		st.UpdatedAt = now
		s.servers[principal] = st
		return s.tasks[i], true
	}
	return codexTaskStatus{}, false
}

func (s *codexTeamState) completeTask(principal contract.ActorID, taskID string, result codexTaskResult) codexTaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	finalState := "passed"
	errText := ""
	if result.Err != nil {
		finalState = "failed"
		errText = result.Err.Error()
	}
	var out codexTaskStatus
	for i := range s.tasks {
		if s.tasks[i].ID != taskID {
			continue
		}
		s.tasks[i].State = finalState
		s.tasks[i].FinishedAt = now
		s.tasks[i].Duration = result.Duration.Round(time.Millisecond).String()
		s.tasks[i].ExitCode = result.ExitCode
		s.tasks[i].Output = result.Output
		s.tasks[i].Error = errText
		out = s.tasks[i]
		break
	}
	st := s.servers[principal]
	st.State = finalState
	st.LastSummary = finalState + " " + taskID
	if errText != "" {
		st.LastError = errText
	} else {
		st.LastError = ""
	}
	st.UpdatedAt = now
	s.servers[principal] = st
	return out
}

func (s *codexTeamState) updateTaskProgress(principal contract.ActorID, taskID string, elapsed time.Duration, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		if s.tasks[i].ID != taskID || s.tasks[i].State != "running" {
			continue
		}
		s.tasks[i].Duration = elapsed.Round(time.Second).String()
		s.tasks[i].Output = output
		break
	}
	st := s.servers[principal]
	st.State = "running"
	st.LastSummary = "running " + taskID + " for " + elapsed.Round(time.Second).String()
	st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.servers[principal] = st
}

func (s *codexTeamState) taskSnapshot() []codexTaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]codexTaskStatus(nil), s.tasks...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *codexTeamState) messageSnapshot() []codexTeamMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]codexTeamMessage(nil), s.messages...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].At == out[j].At {
			return out[i].Principal < out[j].Principal
		}
		return out[i].At < out[j].At
	})
	return out
}

func (s *codexTeamState) eventSnapshot() []codexObservedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]codexObservedEvent(nil), s.events...)
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

func (s *codexTeamState) setProtocolStatus(name, status, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	p := s.protocols[name]
	if p.Name == "" {
		p = codexProtocolStatus{
			Name:         name,
			Source:       "dynamic",
			ObservedType: name + ".write_candidate.observed",
			ProposedType: name + ".write.proposed",
			Resource:     name + "/project",
		}
	}
	p.Status = status
	p.Summary = summary
	p.UpdatedAt = now
	s.protocols[name] = p
}

func (s *codexTeamState) protocolActive(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.protocols[name]
	return p.Status == "active" || p.Status == "used"
}

func (s *codexTeamState) protocolSnapshot() []codexProtocolStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]codexProtocolStatus, 0, len(s.protocols))
	for _, p := range s.protocols {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Name < out[j].Name
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func (s *codexTeamState) mergeProtocolUsage(events []codexObservedEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	latestSummary := map[string]string{}
	latestAt := map[string]string{}
	for _, ev := range events {
		if !strings.HasSuffix(ev.Type, ".write_candidate.observed") {
			continue
		}
		name, _, ok := strings.Cut(ev.Type, ".")
		if !ok {
			continue
		}
		if _, exists := s.protocols[name]; !exists {
			continue
		}
		counts[name]++
		if strings.TrimSpace(ev.Summary) != "" {
			latestSummary[name] = ev.Summary
		}
		if strings.TrimSpace(ev.At) != "" {
			latestAt[name] = ev.At
		}
	}
	for name, count := range counts {
		p := s.protocols[name]
		p.Uses = count
		if p.Source == "dynamic" && count > 0 && (p.Status == "active" || p.Status == "used") {
			p.Status = "used"
		}
		if latestSummary[name] != "" {
			p.Summary = latestSummary[name]
		}
		if latestAt[name] != "" {
			p.UpdatedAt = latestAt[name]
		}
		s.protocols[name] = p
	}
}

func (s *codexTeamState) markProtocolUsedLocked(eventType, summary, now string) {
	name, _, ok := strings.Cut(eventType, ".")
	if !ok {
		return
	}
	p, exists := s.protocols[name]
	if !exists {
		return
	}
	if p.Status == "proposed" || p.Status == "materialized" {
		return
	}
	if strings.HasSuffix(eventType, ".write_candidate.observed") {
		p.Uses++
		if p.Source == "dynamic" {
			p.Status = "used"
		}
		if strings.TrimSpace(summary) != "" {
			p.Summary = summary
		}
		p.UpdatedAt = now
		s.protocols[name] = p
	}
}

func (s *codexTeamState) taskCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

func (s *codexTeamState) taskStateCount(stateName string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, t := range s.tasks {
		if t.State == stateName {
			count++
		}
	}
	return count
}

func codexTeamDynamicProtocols() []codexProtocolStatus {
	now := time.Now().UTC().Format(time.RFC3339)
	return []codexProtocolStatus{
		{
			Name:         "poc_claim",
			Status:       "waiting",
			Source:       "dynamic",
			ObservedType: "poc_claim.write_candidate.observed",
			ProposedType: "poc_claim.write.proposed",
			Resource:     "poc_claim/project",
			Purpose:      "turn coarse progress into reviewable claims with evidence",
			Summary:      "not defined yet",
			UpdatedAt:    now,
		},
		{
			Name:         "poc_decision",
			Status:       "waiting",
			Source:       "dynamic",
			ObservedType: "poc_decision.write_candidate.observed",
			ProposedType: "poc_decision.write.proposed",
			Resource:     "poc_decision/project",
			Purpose:      "record a governed POC-level decision after claims and reviews",
			Summary:      "not defined yet",
			UpdatedAt:    now,
		},
	}
}

func codexTeamDynamicSpecs() map[string]string {
	return map[string]string{
		"poc_claim":    `{"schema_version":1,"name":"poc_claim","observed_type":"poc_claim.write_candidate.observed","proposed_type":"poc_claim.write.proposed","resource_kind":"poc_claim","items_field":"items","fields":[{"name":"claim","validators":[{"id":"required","params":{"missing_style":"empty"}},{"id":"safety:unsafe"}]},{"name":"evidence","validators":[{"id":"required","params":{"missing_style":"empty"}},{"id":"safety:unsafe"}]},{"name":"next_action","validators":[{"id":"safety:unsafe"}]},{"name":"lane","validators":[{"id":"safety:unsafe"}]}],"render":{"content":{"member":"bullet-list","params":{"title":"# POC Claims","field":"claim"}}},"risk":"mid"}`,
		"poc_decision": `{"schema_version":1,"name":"poc_decision","observed_type":"poc_decision.write_candidate.observed","proposed_type":"poc_decision.write.proposed","resource_kind":"poc_decision","items_field":"items","fields":[{"name":"decision","validators":[{"id":"required","params":{"missing_style":"empty"}},{"id":"safety:unsafe"}]},{"name":"rationale","validators":[{"id":"required","params":{"missing_style":"empty"}},{"id":"safety:unsafe"}]},{"name":"evidence","validators":[{"id":"required","params":{"missing_style":"empty"}},{"id":"safety:unsafe"}]},{"name":"followup","validators":[{"id":"safety:unsafe"}]}],"render":{"content":{"member":"bullet-list","params":{"title":"# POC Decisions","field":"decision"}}},"risk":"mid"}`,
	}
}

func runCodexTeamProtocolEvolution(ctx context.Context, runtimeHandle *codexTeamRuntimeHandle, state *codexTeamState, controlURL, operatorToken string) {
	if operatorToken == "" {
		state.setProtocolStatus("poc_claim", "error", "operator token unavailable")
		state.setProtocolStatus("poc_decision", "error", "operator token unavailable")
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}
	operator := contract.ActorID("human@owner")
	client := channel.NewClientWithToken(controlURL, operatorToken)
	specs := codexTeamDynamicSpecs()
	names := []string{"poc_claim", "poc_decision"}
	for _, name := range names {
		state.setProtocolStatus(name, "proposed", "operator submitted "+name+" through loopdef")
		rec, err := client.IngestObserve(operator, contract.ObservationEnvelope{
			ExternalID: state.runID + "-loopdef-" + name,
			Event: contract.Event{
				Type:    "loopdef.write_candidate.observed",
				Payload: map[string]any{"spec": specs[name]},
			},
		})
		state.record(operator, "loopdef.write_candidate.observed", "proposed dynamic event family "+name, rec.Seq, err)
		if err != nil {
			state.setProtocolStatus(name, "error", err.Error())
			return
		}
	}
	materialized, err := runtimeHandle.MaterializeLoopdefs()
	if err != nil {
		for _, name := range names {
			state.setProtocolStatus(name, "error", "materialize failed: "+err.Error())
		}
		return
	}
	for _, name := range materialized {
		if _, ok := specs[name]; ok {
			state.setProtocolStatus(name, "materialized", "wrote .mnemon/loops/"+name+"/capability.json")
		}
	}
	if err := runtimeHandle.ReloadFromDynamicRoot(); err != nil {
		for _, name := range names {
			state.setProtocolStatus(name, "error", "reload failed: "+err.Error())
		}
		return
	}
	for _, name := range names {
		state.setProtocolStatus(name, "active", name+" activated after governed loopdef reload")
	}
	rec, err := client.IngestObserve(operator, contract.ObservationEnvelope{
		ExternalID: state.runID + "-dynamic-protocols-active",
		Event: contract.Event{
			Type: "progress_digest.write_candidate.observed",
			Payload: map[string]any{
				"summary": "dynamic POC protocols active: poc_claim and poc_decision are now governed event families",
			},
		},
	})
	state.record(operator, "progress_digest.write_candidate.observed", "dynamic POC protocols activated", rec.Seq, err)
}

func codexTeamMaterializeLoopdefs(rt *hruntime.Runtime, projectRoot string) ([]string, error) {
	version, fields, err := rt.Resource(contract.ResourceRef{Kind: "loopdef", ID: "project"})
	if err != nil {
		return nil, err
	}
	if version == 0 {
		return nil, nil
	}
	items, _ := fields["items"].([]any)
	var names []string
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		specJSON, _ := item["spec"].(string)
		if strings.TrimSpace(specJSON) == "" {
			continue
		}
		name, err := codexTeamMaterializeDraft(projectRoot, specJSON, version)
		if err != nil {
			return names, err
		}
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func codexTeamMaterializeDraft(projectRoot, specJSON string, loopdefVersion contract.Version) (string, error) {
	var spec map[string]any
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return "", fmt.Errorf("materialize: parse draft: %w", err)
	}
	name, _ := spec["name"].(string)
	if name == "" {
		return "", fmt.Errorf("materialize: draft has no name")
	}
	target := filepath.Join(projectRoot, ".mnemon", "loops", name)
	markerPath := filepath.Join(target, ".managed")
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		if _, merr := os.Stat(markerPath); os.IsNotExist(merr) {
			return "", nil
		}
	}
	spec["default_enabled"] = true
	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(target, "capability.json"), out, 0o600); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(specJSON))
	marker, err := json.Marshal(map[string]any{
		"materialized_by": "codex-team-loopdef",
		"version":         int64(loopdefVersion),
		"digest":          hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(markerPath, marker, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

func runRealCodexAppserver(ctx context.Context, index int, principal contract.ActorID, token, tokenFile, harnessBinary, controlURL string, interval, turnTimeout time.Duration, workDir string, state *codexTeamState, towerBrief func(contract.ActorID, codexTaskStatus) string) {
	client := channel.NewClientWithToken(controlURL, token)
	id := strings.TrimSuffix(string(principal), "@appserver")
	send := func(externalID, eventType, summary string, payload map[string]any) {
		rec, err := client.IngestObserve(principal, contract.ObservationEnvelope{
			ExternalID: externalID,
			Event:      contract.Event{Type: eventType, Payload: payload},
		})
		state.record(principal, eventType, summary, rec.Seq, err)
	}

	if index == 0 {
		send(state.runID+"-"+id+"-intent", "project_intent.write_candidate.observed",
			"declared real Codex appserver collaboration intent",
			map[string]any{"statement": "coordinate five real Codex appservers through Mnemon observations, POC/event-style payloads, and Tower feedback", "evidence": "codex-team-real-appserver"})
	}

	time.Sleep(time.Duration(index%5) * 150 * time.Millisecond)
	task, ok := state.claimTask(principal)
	if !ok {
		state.setAgentState(principal, "idle", "task queue drained")
		return
	}
	taskKey := state.runID + "-" + id + "-" + task.ID
	send(taskKey+"-assignment", "assignment.write_candidate.observed",
		"claimed "+task.ID,
		map[string]any{"scope": task.Title, "ttl": turnTimeout.String(), "assignee": string(principal), "evidence": "codex-team-real-appserver"})

	server := newCodexRealAppServer(codexTeamCodexCmd, workDir)
	if err := server.start(); err != nil {
		result := codexTaskResult{ExitCode: 1, Err: err, Output: server.stderr.String()}
		state.completeTask(principal, task.ID, result)
		state.appendMessage(principal, task.ID, 0, "error", err.Error())
		return
	}
	defer server.close()

	if _, err := server.request("initialize", map[string]any{"clientInfo": map[string]any{"name": "mnemon-codex-team", "version": "0.1.0"}}, 30*time.Second); err != nil {
		result := codexTaskResult{ExitCode: 1, Err: err, Output: server.stderr.String()}
		state.completeTask(principal, task.ID, result)
		state.appendMessage(principal, task.ID, 0, "error", err.Error())
		return
	}
	thread, err := server.request("thread/start", map[string]any{
		"cwd":                   workDir,
		"approvalPolicy":        "never",
		"ephemeral":             true,
		"developerInstructions": codexTeamDeveloperInstructions(principal, task, harnessBinary, controlURL, tokenFile),
	}, 30*time.Second)
	if err != nil {
		result := codexTaskResult{ExitCode: 1, Err: err, Output: server.stderr.String()}
		state.completeTask(principal, task.ID, result)
		state.appendMessage(principal, task.ID, 0, "error", err.Error())
		return
	}
	threadID := codexTeamThreadID(thread)
	if threadID == "" {
		err := fmt.Errorf("thread/start did not return a thread id")
		result := codexTaskResult{ExitCode: 1, Err: err, Output: codexTeamJSON(thread)}
		state.completeTask(principal, task.ID, result)
		state.appendMessage(principal, task.ID, 0, "error", err.Error())
		return
	}
	state.appendMessage(principal, task.ID, 0, "appserver", "started real codex app-server thread "+threadID)

	start := time.Now()
	var finalText string
	for round := 1; round <= codexTeamRounds; round++ {
		if ctx.Err() != nil {
			return
		}
		state.startTaskRound(principal, task.ID, threadID, round)
		prompt := codexTeamRoundPrompt(principal, task, round, towerBrief(principal, task), state.messageSnapshot(), codexTeamObserveCommands(harnessBinary, controlURL, principal, tokenFile, task.ID, round, state.protocolSnapshot()))
		state.appendMessage(principal, task.ID, round, "prompt", "sent round prompt with current Tower snapshot")
		send(fmt.Sprintf("%s-round-%02d-start", taskKey, round), "progress_digest.write_candidate.observed",
			fmt.Sprintf("started %s round %d", task.ID, round),
			map[string]any{"summary": fmt.Sprintf("%s started %s round %d", id, task.ID, round)})

		before := server.notificationCount()
		if _, err := server.request("turn/start", map[string]any{
			"threadId":       threadID,
			"input":          []map[string]any{{"type": "text", "text": prompt}},
			"cwd":            workDir,
			"approvalPolicy": "never",
			"sandboxPolicy":  map[string]any{"type": codexTeamSandbox},
		}, 30*time.Second); err != nil {
			result := codexTaskResult{Duration: time.Since(start), ExitCode: 1, Err: err, Output: server.stderr.String()}
			state.completeTask(principal, task.ID, result)
			state.appendMessage(principal, task.ID, round, "error", err.Error())
			return
		}
		if _, err := server.waitNotification("turn/completed", turnTimeout, before); err != nil {
			result := codexTaskResult{Duration: time.Since(start), ExitCode: 1, Err: err, Output: server.stderr.String()}
			state.completeTask(principal, task.ID, result)
			state.appendMessage(principal, task.ID, round, "error", err.Error())
			return
		}
		notes := server.notificationsSince(before)
		finalText = codexTeamFinalAnswer(notes)
		if finalText == "" {
			finalText = codexTeamTrimOutput(codexTeamCombinedText(notes), 2500)
		}
		activity := codexTeamCommandActivity(notes)
		if activity != "" {
			state.appendMessage(principal, task.ID, round, "activity", activity)
		}
		state.appendMessage(principal, task.ID, round, "final", finalText)
		state.updateTaskProgress(principal, task.ID, time.Since(start), finalText)
		poc := codexTeamExtractPOC(finalText)
		send(fmt.Sprintf("%s-round-%02d-final", taskKey, round), "progress_digest.write_candidate.observed",
			fmt.Sprintf("completed %s round %d", task.ID, round),
			map[string]any{
				"summary": fmt.Sprintf("%s completed %s round %d: %s", id, task.ID, round, codexTeamOneLine(finalText)),
				"poc":     poc,
			})
		if state.protocolActive("poc_claim") {
			payload := codexTeamPOCClaimPayload(task, poc, finalText)
			send(fmt.Sprintf("%s-round-%02d-poc-claim", taskKey, round), "poc_claim.write_candidate.observed",
				"published governed POC claim for "+task.ID, payload)
		}
		if state.protocolActive("poc_decision") && codexTeamShouldEmitDecision(task, round) {
			payload := codexTeamPOCDecisionPayload(task, poc, finalText)
			send(fmt.Sprintf("%s-round-%02d-poc-decision", taskKey, round), "poc_decision.write_candidate.observed",
				"published governed POC decision from "+task.ID, payload)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
	send(taskKey+"-memory", "memory.write_candidate.observed",
		"published real Codex appserver lane memory for "+task.ID,
		map[string]any{"content": fmt.Sprintf("%s final synthesis from %s: %s", task.ID, principal, codexTeamOneLine(finalText)), "source": "codex-team-real-appserver", "confidence": "medium"})
	state.completeTask(principal, task.ID, codexTaskResult{Duration: time.Since(start), ExitCode: 0, Output: finalText})
}

func runShellCodexAppserver(ctx context.Context, index int, principal contract.ActorID, token, controlURL string, interval, taskTimeout time.Duration, workDir string, state *codexTeamState) {
	client := channel.NewClientWithToken(controlURL, token)
	id := strings.TrimSuffix(string(principal), "@appserver")
	send := func(externalID, eventType, summary string, payload map[string]any) {
		rec, err := client.IngestObserve(principal, contract.ObservationEnvelope{
			ExternalID: externalID,
			Event:      contract.Event{Type: eventType, Payload: payload},
		})
		state.record(principal, eventType, summary, rec.Seq, err)
	}

	if index == 0 {
		send(state.runID+"-"+id+"-intent", "project_intent.write_candidate.observed",
			"declared shared project intent",
			map[string]any{"statement": "coordinate five Codex appservers through Mnemon while executing real local tasks", "evidence": "codex-team-demo"})
	}

	time.Sleep(time.Duration(index%5) * 150 * time.Millisecond)
	for {
		if ctx.Err() != nil {
			return
		}
		task, ok := state.claimTask(principal)
		if !ok {
			state.setAgentState(principal, "idle", "task queue drained")
			return
		}
		taskKey := state.runID + "-" + id + "-" + task.ID
		send(taskKey+"-assignment", "assignment.write_candidate.observed",
			"claimed "+task.ID,
			map[string]any{"scope": task.Title, "ttl": taskTimeout.String(), "assignee": string(principal), "evidence": "codex-team-demo"})
		send(taskKey+"-start", "progress_digest.write_candidate.observed",
			"started "+task.ID,
			map[string]any{"summary": fmt.Sprintf("%s started %s: %s", id, task.ID, task.Command)})

		progressSeq := 0
		result := runCodexTeamTask(ctx, task, taskTimeout, workDir, codexTeamProgressEvery(interval), func(elapsed time.Duration, output string) {
			progressSeq++
			state.updateTaskProgress(principal, task.ID, elapsed, output)
			send(fmt.Sprintf("%s-tick-%03d", taskKey, progressSeq), "progress_digest.write_candidate.observed",
				"advanced "+task.ID,
				map[string]any{"summary": fmt.Sprintf("%s running %s for %s: %s", id, task.ID, elapsed.Round(time.Second), codexTeamOneLine(output))})
		})
		stateText := "completed"
		if result.Err != nil {
			stateText = "failed"
		}
		send(taskKey+"-finish", "progress_digest.write_candidate.observed",
			stateText+" "+task.ID,
			map[string]any{"summary": fmt.Sprintf("%s %s %s in %s: %s", id, stateText, task.ID, result.Duration.Round(time.Millisecond), codexTeamOneLine(result.Output))})
		if result.Err == nil {
			send(taskKey+"-memory", "memory.write_candidate.observed",
				"published result memory for "+task.ID,
				map[string]any{"content": fmt.Sprintf("%s passed command %q in %s", task.ID, task.Command, result.Duration.Round(time.Millisecond)), "source": "codex-team-demo", "confidence": "high"})
		}
		state.completeTask(principal, task.ID, result)

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func runCodexTeamTask(parent context.Context, task codexTaskStatus, timeout time.Duration, workDir string, progressEvery time.Duration, onProgress func(time.Duration, string)) codexTaskResult {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var output lockedOutput
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", task.Command)
	cmd.Dir = workDir
	cmd.Stdout = &output
	cmd.Stderr = &output

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return codexTaskResult{Duration: time.Since(start), ExitCode: 1, Output: output.String(), Err: err}
	}
	waitc := make(chan error, 1)
	go func() { waitc <- cmd.Wait() }()

	var err error
	ticker := time.NewTicker(progressEvery)
	defer ticker.Stop()
	for {
		select {
		case err = <-waitc:
			goto done
		case <-ticker.C:
			if onProgress != nil {
				onProgress(time.Since(start), codexTeamTrimOutput(output.String(), 4000))
			}
		}
	}

done:
	duration := time.Since(start)
	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("task timed out after %s", timeout)
		} else if ctx.Err() == context.Canceled {
			err = context.Canceled
		}
	}
	return codexTaskResult{
		Duration: duration,
		ExitCode: exitCode,
		Output:   codexTeamTrimOutput(output.String(), 4000),
		Err:      err,
	}
}

type codexRealAppServer struct {
	command       string
	cwd           string
	proc          *exec.Cmd
	stdin         io.WriteCloser
	messages      chan map[string]any
	responses     map[int]map[string]any
	notifications []map[string]any
	nextID        int
	stderr        lockedOutput
}

func newCodexRealAppServer(command, cwd string) *codexRealAppServer {
	return &codexRealAppServer{
		command:   command,
		cwd:       cwd,
		messages:  make(chan map[string]any, 256),
		responses: map[int]map[string]any{},
		nextID:    1,
	}
}

func (s *codexRealAppServer) start() error {
	cmd := exec.Command(s.command, "app-server", "--listen", "stdio://")
	cmd.Dir = s.cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.proc = cmd
	s.stdin = stdin
	go s.readStdout(stdout)
	go func() { _, _ = io.Copy(&s.stderr, stderr) }()
	return nil
}

func (s *codexRealAppServer) close() {
	if s.proc == nil || s.proc.Process == nil {
		return
	}
	if s.proc.ProcessState != nil && s.proc.ProcessState.Exited() {
		return
	}
	_ = s.proc.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_ = s.proc.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = s.proc.Process.Kill()
		<-done
	}
}

func (s *codexRealAppServer) readStdout(stdout io.Reader) {
	defer close(s.messages)
	reader := bufio.NewReaderSize(stdout, 1024*1024)
	for {
		line, err := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			var msg map[string]any
			if jerr := json.Unmarshal([]byte(line), &msg); jerr == nil {
				s.messages <- msg
			} else {
				s.messages <- map[string]any{"method": "mnemon/invalid-json", "params": map[string]any{"line": line, "error": jerr.Error()}}
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *codexRealAppServer) request(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	if s.stdin == nil {
		return nil, fmt.Errorf("codex app-server is not running")
	}
	id := s.nextID
	s.nextID++
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := s.stdin.Write(append(data, '\n')); err != nil {
		return nil, err
	}
	return s.waitResponse(id, timeout)
}

func (s *codexRealAppServer) waitResponse(id int, timeout time.Duration) (map[string]any, error) {
	deadline := time.After(timeout)
	for {
		if resp, ok := s.responses[id]; ok {
			delete(s.responses, id)
			if raw, ok := resp["error"]; ok {
				return nil, fmt.Errorf("codex app-server error: %s", codexTeamJSON(raw))
			}
			if result, ok := resp["result"].(map[string]any); ok {
				return result, nil
			}
			return map[string]any{}, nil
		}
		select {
		case <-deadline:
			return nil, fmt.Errorf("timed out waiting for response id %d", id)
		case msg, ok := <-s.messages:
			if !ok {
				return nil, fmt.Errorf("codex app-server stdout closed: %s", s.stderr.String())
			}
			s.acceptMessage(msg)
		}
	}
}

func (s *codexRealAppServer) waitNotification(method string, timeout time.Duration, startIndex int) (map[string]any, error) {
	deadline := time.After(timeout)
	cursor := startIndex
	if cursor < 0 || cursor > len(s.notifications) {
		cursor = len(s.notifications)
	}
	for {
		for cursor < len(s.notifications) {
			n := s.notifications[cursor]
			cursor++
			if n["method"] == method {
				return n, nil
			}
		}
		select {
		case <-deadline:
			return nil, fmt.Errorf("timed out waiting for notification %s", method)
		case msg, ok := <-s.messages:
			if !ok {
				return nil, fmt.Errorf("codex app-server stdout closed: %s", s.stderr.String())
			}
			s.acceptMessage(msg)
		}
	}
}

func (s *codexRealAppServer) acceptMessage(msg map[string]any) {
	if id, ok := codexTeamMessageID(msg); ok {
		s.responses[id] = msg
		return
	}
	s.notifications = append(s.notifications, msg)
}

func (s *codexRealAppServer) notificationCount() int {
	return len(s.notifications)
}

func (s *codexRealAppServer) notificationsSince(index int) []map[string]any {
	if index < 0 || index > len(s.notifications) {
		index = len(s.notifications)
	}
	return append([]map[string]any(nil), s.notifications[index:]...)
}

func codexTeamMessageID(msg map[string]any) (int, bool) {
	raw, ok := msg["id"]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

type lockedOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *lockedOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *lockedOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func codexTeamProgressEvery(interval time.Duration) time.Duration {
	if interval < 3*time.Second {
		return 3 * time.Second
	}
	return interval
}

func codexTeamTaskSpecs(raw []string, backend string) ([]codexTaskSpec, error) {
	if len(raw) == 0 {
		return defaultCodexTeamTasks(backend), nil
	}
	out := make([]codexTaskSpec, 0, len(raw))
	seen := map[string]bool{}
	for i, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("--task %d is empty", i+1)
		}
		id, command, ok := strings.Cut(item, "=")
		if !ok {
			id = fmt.Sprintf("task-%02d", i+1)
			command = item
		}
		id = codexTeamTaskID(id, i+1)
		command = strings.TrimSpace(command)
		if command == "" {
			return nil, fmt.Errorf("--task %s has an empty command", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate task id %q", id)
		}
		seen[id] = true
		out = append(out, codexTaskSpec{ID: id, Title: id, Command: command})
	}
	return out, nil
}

func defaultCodexTeamTasks(backend string) []codexTaskSpec {
	if backend == "codex" {
		return []codexTaskSpec{
			{ID: "protocol-gap", Title: "Protocol gap", Command: "Explain why progress_digest is too coarse to prove real collaboration. Watch for poc_claim activation, then turn your finding into a reviewable claim. Do not modify files."},
			{ID: "claim-model", Title: "Claim model", Command: "Inspect capability-spec and loopdef behavior. Judge whether poc_claim fits the existing dynamic event family model, citing repo evidence. Do not modify files."},
			{ID: "runtime-governance", Title: "Runtime governance", Command: "Adversarially review whether defining poc_claim/poc_decision through loopdef preserves the observe/propose/kernel trust boundary. Do not modify files."},
			{ID: "agent-routing", Title: "Agent routing", Command: "Design how personalized Tower views should route poc_claim records to reviewer/tester/synthesizer lanes. React to other lanes' claims when they appear. Do not modify files."},
			{ID: "handoff-synthesis", Title: "Handoff synthesis", Command: "Synthesize the activated protocol records into one POC decision: what changed after dynamic event families became active, and what should be built next. Do not modify files."},
		}
	}
	return []codexTaskSpec{
		{ID: "root-build", Title: "Build mnemon CLI", Command: "go build -o /tmp/mnemon-codex-team-root ."},
		{ID: "harness-build", Title: "Build mnemon-harness CLI", Command: "go build -o /tmp/mnemon-codex-team-harness ./harness/cmd/mnemon-harness"},
		{ID: "harness-cmd-tests", Title: "Test harness command package", Command: "go test ./harness/cmd/mnemon-harness"},
		{ID: "harness-app-tests", Title: "Test harness app package", Command: "go test ./harness/internal/app"},
		{ID: "harness-channel-tests", Title: "Test harness channel package", Command: "go test ./harness/internal/channel"},
	}
}

func codexTeamTaskID(raw string, fallback int) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return fmt.Sprintf("task-%02d", fallback)
	}
	return id
}

func codexTeamTrimOutput(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return "... " + string(runes[len(runes)-maxRunes:])
}

func codexTeamOneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no output"
	}
	lines := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return codexTeamTrimOutput(line, 240)
		}
	}
	return "no output"
}

func codexTeamDeveloperInstructions(principal contract.ActorID, task codexTaskStatus, harnessBinary, controlURL, tokenFile string) string {
	return fmt.Sprintf(`You are %s, one real Codex app-server participating in a Mnemon AgentTeam POC.

Your lane is %s.

Rules:
- Do not modify files. Inspect, reason, and report.
- Treat Mnemon as the coordination substrate: observations become progress_digest, assignment, memory, and Tower entries.
- Watch for dynamic protocol activation. When poc_claim or poc_decision becomes active, prefer those event families for reviewable claims and decisions.
- Each turn should produce operator-readable progress, not raw logs.
- Include a compact POC_EVENT block at the end with keys: event_type, claim, evidence, next_action.
- Be concrete: cite files, commands, or observed Tower entries when possible.
- You may directly emit a Mnemon observation with %s control observe --addr %s --principal %s --token-file %s.
- The token file path is a credential handle; use it only with --token-file and never print its contents.`, principal, task.Title, harnessBinary, controlURL, principal, tokenFile)
}

func codexTeamRoundPrompt(principal contract.ActorID, task codexTaskStatus, round int, tower string, messages []codexTeamMessage, observeCommand string) string {
	recent := codexTeamRecentMessages(messages, string(principal), 10)
	return fmt.Sprintf(`You are %s.

Lane: %s
Round: %d of %d

Lane objective:
%s

Current Mnemon Tower snapshot:
%s

Recent cross-agent messages:
%s

Direct Mnemon observation command(s) for this round:
%s

Work for this round:
1. Read enough repo context to make real progress on your lane.
2. React to the Tower and other agents' latest observations.
3. If you have a concrete claim, run one direct Mnemon observation command with a concise JSON payload before final answer.
4. End with:

POC_EVENT:
event_type: <the most specific active POC event type>
claim: <one concrete claim from this round>
evidence: <file/command/Tower evidence>
next_action: <what another appserver or next round should do>

Do not modify files.`, principal, task.Title, round, task.Rounds, task.Command, tower, recent, observeCommand)
}

func codexTeamObserveCommands(harnessBinary, controlURL string, principal contract.ActorID, tokenFile, taskID string, round int, protocols []codexProtocolStatus) string {
	payload := fmt.Sprintf(`{"summary":"%s round %d direct observation: <replace with your concrete claim>","source":"real-codex-appserver","task":"%s","round":%d}`, taskID, round, taskID, round)
	commands := []string{
		fmt.Sprintf("%s control observe --addr %s --principal %s --token-file %s --type progress_digest.write_candidate.observed --external-id direct-%s-round-%02d --payload '%s'",
			harnessBinary, controlURL, principal, tokenFile, taskID, round, payload),
	}
	if codexTeamProtocolIsActive(protocols, "poc_claim") {
		claimPayload := fmt.Sprintf(`{"claim":"<one reviewable claim>","evidence":"<file/command/Tower evidence>","next_action":"<who should react next>","lane":"%s"}`, taskID)
		commands = append(commands, fmt.Sprintf("%s control observe --addr %s --principal %s --token-file %s --type poc_claim.write_candidate.observed --external-id direct-%s-claim-%02d --payload '%s'",
			harnessBinary, controlURL, principal, tokenFile, taskID, round, claimPayload))
	}
	if codexTeamProtocolIsActive(protocols, "poc_decision") && strings.Contains(taskID, "handoff") {
		decisionPayload := `{"decision":"<POC decision>","rationale":"<why this decision follows from claims>","evidence":"<accepted claims/Tower evidence>","followup":"<next implementation step>"}`
		commands = append(commands, fmt.Sprintf("%s control observe --addr %s --principal %s --token-file %s --type poc_decision.write_candidate.observed --external-id direct-%s-decision-%02d --payload '%s'",
			harnessBinary, controlURL, principal, tokenFile, taskID, round, decisionPayload))
	}
	return strings.Join(commands, "\n")
}

func codexTeamProtocolIsActive(protocols []codexProtocolStatus, name string) bool {
	for _, p := range protocols {
		if p.Name == name && (p.Status == "active" || p.Status == "used") {
			return true
		}
	}
	return false
}

func codexTeamTowerBrief(view app.TowerView, principal contract.ActorID, task codexTaskStatus, protocols []codexProtocolStatus) string {
	var b strings.Builder
	b.WriteString("PERSONAL FOCUS:\n")
	b.WriteString(fmt.Sprintf("- actor=%s lane=%s round=%d/%d\n", principal, task.ID, task.Round, task.Rounds))
	b.WriteString("- read the shared state, then react from your lane's responsibility\n")
	b.WriteString("GOAL:\n")
	if len(view.Goal.Statements) == 0 {
		b.WriteString("- none\n")
	}
	for _, s := range view.Goal.Statements {
		b.WriteString("- " + s + "\n")
	}
	b.WriteString("FIELD:\n")
	for _, a := range view.Field.Assignments {
		b.WriteString(fmt.Sprintf("- %s -> %s ttl=%s\n", a.Scope, a.Assignee, a.TTL))
	}
	b.WriteString("RECENT PROGRESS:\n")
	progress := view.Goal.Progress
	if len(progress) > 12 {
		progress = progress[len(progress)-12:]
	}
	if len(progress) == 0 {
		b.WriteString("- none\n")
	}
	for _, p := range progress {
		b.WriteString("- " + p + "\n")
	}
	b.WriteString("ACTIVE / EVOLVING PROTOCOLS:\n")
	for _, p := range protocols {
		if p.Source == "dynamic" || p.Name == "loopdef" {
			b.WriteString(fmt.Sprintf("- %s [%s]: %s\n", p.Name, p.Status, p.Summary))
		}
	}
	b.WriteString(fmt.Sprintf("INBOX: %d open escalation(s)\n", len(view.Inbox.Escalations)))
	return b.String()
}

func codexTeamRecentMessages(messages []codexTeamMessage, self string, limit int) string {
	var rows []string
	for i := len(messages) - 1; i >= 0 && len(rows) < limit; i-- {
		m := messages[i]
		if m.Kind == "prompt" {
			continue
		}
		rows = append(rows, fmt.Sprintf("- %s %s/%s round %d: %s", m.At, m.Principal, m.TaskID, m.Round, codexTeamOneLine(m.Text)))
	}
	if len(rows) == 0 {
		return "- none yet"
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	_ = self
	return strings.Join(rows, "\n")
}

func codexTeamThreadID(result map[string]any) string {
	if thread, ok := result["thread"].(map[string]any); ok {
		if id, ok := thread["id"].(string); ok {
			return id
		}
	}
	if id, ok := result["threadId"].(string); ok {
		return id
	}
	if id, ok := result["id"].(string); ok {
		return id
	}
	return ""
}

func codexTeamFinalAnswer(notifications []map[string]any) string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["type"] == "agentMessage" && x["phase"] == "final_answer" {
				if text, ok := x["text"].(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, text)
				}
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	for _, n := range notifications {
		walk(n)
	}
	return strings.TrimSpace(strings.Join(out, "\n\n"))
}

func codexTeamCommandActivity(notifications []map[string]any) string {
	count := 0
	var commands []string
	for _, n := range notifications {
		text := codexTeamCombinedText([]map[string]any{n})
		if strings.Contains(text, "commandExecution") || strings.Contains(text, "item/started") || strings.Contains(text, "item/completed") {
			count++
		}
		candidates := codexTeamCommandCandidates(n)
		for _, c := range candidates {
			if c != "" {
				commands = append(commands, c)
			}
		}
	}
	if count == 0 && len(commands) == 0 {
		return ""
	}
	if len(commands) > 4 {
		commands = commands[len(commands)-4:]
	}
	if len(commands) == 0 {
		return fmt.Sprintf("Codex app-server emitted %d activity notification(s).", count)
	}
	return fmt.Sprintf("Codex app-server emitted %d activity notification(s). Commands: %s", count, strings.Join(commands, " | "))
}

func codexTeamCommandCandidates(value any) []string {
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, key := range []string{"command", "cmd", "script"} {
				if s, ok := x[key].(string); ok && strings.TrimSpace(s) != "" && len(s) < 300 {
					out = append(out, strings.TrimSpace(s))
				}
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func codexTeamCombinedText(values []map[string]any) string {
	var parts []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			parts = append(parts, x)
		case map[string]any:
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	for _, v := range values {
		walk(v)
	}
	return strings.Join(parts, "\n")
}

func codexTeamExtractPOC(text string) map[string]any {
	out := map[string]any{}
	lines := strings.Split(text, "\n")
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(strings.TrimSuffix(trimmed, ":"), "POC_EVENT") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(key, " ", "_")))
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func codexTeamPOCClaimPayload(task codexTaskStatus, poc map[string]any, finalText string) map[string]any {
	claim := codexTeamPOCString(poc, "claim")
	if claim == "" {
		claim = codexTeamOneLine(finalText)
	}
	evidence := codexTeamPOCString(poc, "evidence")
	if evidence == "" {
		evidence = "final answer from " + task.ID
	}
	nextAction := codexTeamPOCString(poc, "next_action")
	if nextAction == "" {
		nextAction = "another lane should review or reuse this claim"
	}
	return map[string]any{
		"claim":       claim,
		"evidence":    evidence,
		"next_action": nextAction,
		"lane":        task.ID,
	}
}

func codexTeamPOCDecisionPayload(task codexTaskStatus, poc map[string]any, finalText string) map[string]any {
	decision := codexTeamPOCString(poc, "claim")
	if decision == "" {
		decision = "adopt the activated POC protocol for the next collaboration iteration"
	}
	rationale := codexTeamPOCString(poc, "evidence")
	if rationale == "" {
		rationale = codexTeamOneLine(finalText)
	}
	followup := codexTeamPOCString(poc, "next_action")
	if followup == "" {
		followup = "turn personalized Tower and protocol routing into the next demo slice"
	}
	return map[string]any{
		"decision":  decision,
		"rationale": rationale,
		"evidence":  "handoff lane " + task.ID + " synthesized active poc_claim records",
		"followup":  followup,
	}
}

func codexTeamPOCString(poc map[string]any, key string) string {
	if s, ok := poc[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func codexTeamShouldEmitDecision(task codexTaskStatus, round int) bool {
	return strings.Contains(task.ID, "handoff") && round == task.Rounds
}

func codexTeamJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func parseCodexTeamTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now()
	}
	return t
}

func codexTeamBindings(n int, endpoint string) ([]channel.ChannelBinding, map[string]contract.ActorID, error) {
	refs := []contract.ResourceRef{
		{Kind: "memory", ID: "project"},
		{Kind: "project_intent", ID: "project"},
		{Kind: "assignment", ID: "project"},
		{Kind: "progress_digest", ID: "project"},
		{Kind: "loopdef", ID: "project"},
	}
	observed := []string{
		"session.observed",
		"memory.write_candidate.observed",
		"project_intent.write_candidate.observed",
		"assignment.write_candidate.observed",
		"progress_digest.write_candidate.observed",
		"loopdef.write_candidate.observed",
	}
	bindings := make([]channel.ChannelBinding, 0, n+1)
	tokens := make(map[string]contract.ActorID, n+1)
	for i := 1; i <= n; i++ {
		principal := contract.ActorID(fmt.Sprintf("codex-%02d@appserver", i))
		b := channel.HostAgentBinding(principal, endpoint, refs)
		b.AllowedObservedTypes = observed
		bindings = append(bindings, b)
		tok, err := randomToken()
		if err != nil {
			return nil, nil, err
		}
		tokens[tok] = principal
	}
	operator := channel.ControlAgentBinding("human@owner", endpoint, refs)
	operator.AllowedObservedTypes = observed
	bindings = append(bindings, operator)
	tok, err := randomToken()
	if err != nil {
		return nil, nil, err
	}
	tokens[tok] = "human@owner"
	return bindings, tokens, nil
}

func codexTeamTokenForPrincipal(tokens map[string]contract.ActorID, principal contract.ActorID) string {
	for tok, p := range tokens {
		if p == principal {
			return tok
		}
	}
	return ""
}

func codexTeamWriteTokenFiles(tokens map[string]contract.ActorID) (map[contract.ActorID]string, func(), error) {
	dir, err := os.MkdirTemp("", "mnemon-codex-team-tokens-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	out := map[contract.ActorID]string{}
	for tok, principal := range tokens {
		name := codexTeamTaskID(string(principal), len(out)+1) + ".token"
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(tok+"\n"), 0600); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		out[principal] = path
	}
	return out, cleanup, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func listenerURL(ln net.Listener) string {
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "http://" + ln.Addr().String()
	}
	if host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func codexTeamRole(principal string) string {
	switch {
	case strings.Contains(principal, "01"):
		return "planner"
	case strings.Contains(principal, "02"):
		return "builder"
	case strings.Contains(principal, "03"):
		return "reviewer"
	case strings.Contains(principal, "04"):
		return "tester"
	case strings.Contains(principal, "05"):
		return "integrator"
	default:
		return "operator"
	}
}

func codexTeamScope(index int) string {
	scopes := []string{"plan the work", "implement the change", "review risk", "write handoff", "verify behavior"}
	if index < 0 {
		index = 0
	}
	return scopes[index%len(scopes)]
}

var codexTeamHTML = template.Must(template.New("codex-team").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Mnemon Collaboration Run</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f7f8fb;
      --ink: #17202a;
      --muted: #637083;
      --line: #d9dee8;
      --panel: #ffffff;
      --green: #16835b;
      --blue: #2f6fbd;
      --red: #b44242;
      --amber: #9a6700;
      --violet: #6b5aa6;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--ink);
      letter-spacing: 0;
    }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 18px 24px;
      border-bottom: 1px solid var(--line);
      background: #ffffff;
    }
    h1 {
      margin: 0;
      font-size: 20px;
      font-weight: 700;
    }
    .sub {
      margin-top: 2px;
      color: var(--muted);
      font-size: 13px;
    }
    .statusbar {
      display: grid;
      grid-template-columns: repeat(7, minmax(100px, 1fr));
      gap: 1px;
      background: var(--line);
      border-bottom: 1px solid var(--line);
    }
    .metric {
      min-height: 74px;
      padding: 14px 18px;
      background: #ffffff;
    }
    .metric b {
      display: block;
      font-size: 22px;
      line-height: 1.1;
    }
    .metric span {
      color: var(--muted);
      font-size: 12px;
      text-transform: uppercase;
    }
    main {
      display: grid;
      grid-template-columns: minmax(280px, 380px) 1fr;
      gap: 18px;
      padding: 18px;
    }
    .side {
      display: grid;
      align-content: start;
      gap: 18px;
      min-width: 0;
    }
    .maincol {
      display: grid;
      align-content: start;
      gap: 18px;
      min-width: 0;
    }
    section {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
      min-width: 0;
    }
    section h2 {
      margin: 0;
      padding: 14px 16px;
      font-size: 14px;
      border-bottom: 1px solid var(--line);
    }
    .agents {
      display: grid;
      gap: 10px;
      padding: 12px;
    }
    .agent, .task {
      border-left: 4px solid var(--blue);
      padding: 10px 10px 10px 12px;
      background: #f9fbff;
      border-radius: 6px;
    }
    .agent.error { border-left-color: var(--red); background: #fff8f8; }
    .agent.running, .task.running { border-left-color: var(--amber); background: #fffdf5; }
    .agent.passed, .task.passed { border-left-color: var(--green); background: #f7fffb; }
    .agent.failed, .task.failed { border-left-color: var(--red); background: #fff8f8; }
    .agent.idle { border-left-color: var(--muted); background: #fafafa; }
    .agent-head, .task-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      font-size: 13px;
      font-weight: 650;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 22px;
      padding: 0 8px;
      border-radius: 999px;
      font-size: 12px;
      color: #ffffff;
      background: var(--green);
      white-space: nowrap;
    }
    .pill.running { background: var(--amber); }
    .pill.idle { background: var(--muted); }
    .pill.pending { background: var(--blue); }
    .pill.passed { background: var(--green); }
    .pill.failed { background: var(--red); }
    .pill.error { background: var(--red); }
    .pill.active, .pill.used { background: var(--green); }
    .pill.proposed, .pill.materialized { background: var(--amber); }
    .pill.waiting { background: var(--muted); }
    .agent p, .task p {
      margin: 8px 0 0;
      color: var(--muted);
      font-size: 13px;
      line-height: 1.35;
    }
    .tasks {
      display: grid;
      gap: 10px;
      padding: 12px;
    }
    .protocols {
      display: grid;
      gap: 10px;
      padding: 12px;
    }
    .protocol {
      border: 1px solid var(--line);
      border-left: 4px solid var(--muted);
      border-radius: 8px;
      background: #ffffff;
      padding: 10px 12px;
    }
    .protocol.active, .protocol.used { border-left-color: var(--green); background: #f7fffb; }
    .protocol.proposed, .protocol.materialized { border-left-color: var(--amber); background: #fffdf5; }
    .protocol.waiting { border-left-color: var(--muted); background: #fafafa; }
    .protocol.error { border-left-color: var(--red); background: #fff8f8; }
    .protocol-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      font-size: 13px;
      font-weight: 650;
    }
    .protocol p {
      margin: 8px 0 0;
      color: var(--muted);
      font-size: 13px;
      line-height: 1.4;
    }
    .objective {
      padding: 16px;
      display: grid;
      gap: 12px;
    }
    .objective b {
      display: block;
      font-size: 15px;
      line-height: 1.35;
    }
    .objective span {
      color: var(--muted);
      font-size: 13px;
      line-height: 1.4;
    }
    .flow {
      display: grid;
      grid-template-columns: repeat(4, minmax(0, 1fr));
      gap: 1px;
      background: var(--line);
      border-top: 1px solid var(--line);
    }
    .flow-step {
      min-height: 96px;
      padding: 14px;
      background: #ffffff;
    }
    .flow-step b {
      display: block;
      margin-bottom: 6px;
      font-size: 13px;
    }
    .flow-step span {
      color: var(--muted);
      font-size: 12px;
      line-height: 1.35;
    }
    .worktext {
      display: block;
      margin-top: 8px;
      color: #243244;
      font-size: 13px;
      line-height: 1.4;
      overflow-wrap: anywhere;
    }
    .pre {
      white-space: pre-wrap;
      max-height: 160px;
      overflow: auto;
      color: #243244;
      background: #f5f7fa;
      border: 1px solid #e5e9f0;
      border-radius: 6px;
      padding: 8px;
    }
    .stream {
      display: grid;
      gap: 10px;
      padding: 12px;
      max-height: 460px;
      overflow: auto;
    }
    .message, .event {
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 10px 12px;
      background: #ffffff;
    }
    .message-head, .event-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      color: var(--muted);
      font-size: 12px;
    }
    .message strong, .event strong {
      color: var(--ink);
      font-size: 13px;
    }
    .message p, .event p {
      margin: 8px 0 0;
      color: #243244;
      font-size: 13px;
      line-height: 1.4;
      white-space: pre-wrap;
      overflow-wrap: anywhere;
    }
    .attention {
      display: grid;
      gap: 10px;
      padding: 12px;
    }
    .attention-item {
      border-left: 4px solid var(--amber);
      border-radius: 6px;
      background: #fffdf5;
      padding: 10px 12px;
      font-size: 13px;
      line-height: 1.4;
    }
    .attention-item.clear {
      border-left-color: var(--green);
      background: #f7fffb;
      color: var(--muted);
    }
    details.evidence {
      border-top: 1px solid var(--line);
    }
    details.evidence summary {
      cursor: pointer;
      padding: 14px 16px;
      color: var(--muted);
      font-size: 13px;
      user-select: none;
    }
    .tower {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 14px;
      padding: 14px;
    }
    .page {
      min-height: 220px;
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      background: #ffffff;
    }
    .page h3 {
      margin: 0;
      padding: 12px 14px;
      font-size: 13px;
      border-bottom: 1px solid var(--line);
      background: #fbfcfe;
    }
    .page.goal h3 { color: var(--green); }
    .page.field h3 { color: var(--blue); }
    .page.inbox h3 { color: var(--amber); }
    .page.ledger h3 { color: var(--violet); }
    ul {
      list-style: none;
      margin: 0;
      padding: 10px 14px 14px;
      display: grid;
      gap: 8px;
    }
    li {
      min-height: 28px;
      color: #243244;
      font-size: 13px;
      line-height: 1.35;
      overflow-wrap: anywhere;
    }
    .empty {
      color: var(--muted);
      font-style: italic;
    }
    code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      font-size: 12px;
      background: #eef2f7;
      padding: 2px 4px;
      border-radius: 4px;
    }
    @media (max-width: 900px) {
      main, .tower, .statusbar, .flow { grid-template-columns: 1fr; }
      header { align-items: flex-start; flex-direction: column; }
    }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Mnemon Collaboration Run</h1>
      <div class="sub" id="endpoint">Connecting...</div>
    </div>
    <div class="sub" id="clock">--</div>
  </header>
  <div class="statusbar">
    <div class="metric"><b id="m-agents">0</b><span>agents</span></div>
    <div class="metric"><b id="m-tasks">0</b><span>lanes</span></div>
    <div class="metric"><b id="m-running">0</b><span>active</span></div>
    <div class="metric"><b id="m-passed">0</b><span>complete</span></div>
    <div class="metric"><b id="m-inbox">0</b><span>attention</span></div>
    <div class="metric"><b id="m-decisions">0</b><span>decisions</span></div>
    <div class="metric"><b id="m-protocols">0</b><span>protocols</span></div>
  </div>
  <main>
    <div class="side">
      <section>
        <h2>Team</h2>
        <div class="agents" id="agents"></div>
      </section>
      <section>
        <h2>Work Lanes</h2>
        <div class="tasks" id="tasks"></div>
      </section>
    </div>
    <div class="maincol">
      <section>
        <h2>Shared Objective</h2>
        <div class="objective" id="objective"></div>
        <div class="flow">
          <div class="flow-step"><b>1. Notice gap</b><span>Progress digests are useful but too coarse for reviewable claims.</span></div>
          <div class="flow-step"><b>2. Define protocol</b><span>Operator submits loopdef candidates for new event families.</span></div>
          <div class="flow-step"><b>3. Reload catalog</b><span>Mnemon materializes and governs the new families.</span></div>
          <div class="flow-step"><b>4. Use it</b><span>Agents emit poc_claim and poc_decision records.</span></div>
        </div>
      </section>
      <section>
        <h2>Protocol Evolution</h2>
        <div class="protocols" id="protocols"></div>
      </section>
      <section>
        <h2>Collaboration Stream</h2>
        <div class="stream" id="messages"></div>
      </section>
      <section>
        <h2>Needs Attention</h2>
        <div class="attention" id="attention"></div>
      </section>
      <section>
        <h2>Evidence Trail</h2>
        <details class="evidence">
          <summary>Show governed records and low-level proof</summary>
          <div class="tower">
            <div class="page goal"><h3>Shared Goal</h3><ul id="goal"></ul></div>
            <div class="page field"><h3>Current Field</h3><ul id="field"></ul></div>
            <div class="page inbox"><h3>Escalations</h3><ul id="inbox"></ul></div>
            <div class="page ledger"><h3>Decisions</h3><ul id="ledger"></ul></div>
          </div>
          <div class="stream" id="events"></div>
        </details>
      </section>
    </div>
  </main>
  <script>
    const text = value => String(value ?? "");
	    const li = (html, cls = "") => '<li class="' + cls + '">' + html + '</li>';
	    const esc = value => text(value).replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
	    const code = value => '<code>' + esc(value) + '</code>';
    const clip = (value, n = 700) => {
      const s = text(value).trim();
      return s.length > n ? "..." + s.slice(-n) : s;
    };
    function list(target, rows, empty) {
      document.getElementById(target).innerHTML = rows.length ? rows.join("") : li(esc(empty), "empty");
    }
    async function refresh() {
      const res = await fetch("/api/snapshot", {cache: "no-store"});
      const data = await res.json();
      document.getElementById("endpoint").textContent = data.control_url + " - " + data.uptime + " - " + data.store_path;
      document.getElementById("clock").textContent = data.now;
      document.getElementById("m-agents").textContent = data.counts.agents;
      document.getElementById("m-tasks").textContent = data.counts.tasks;
      document.getElementById("m-running").textContent = data.counts.running;
      document.getElementById("m-passed").textContent = data.counts.passed;
      document.getElementById("m-inbox").textContent = data.counts.inbox;
      document.getElementById("m-decisions").textContent = data.counts.ledger;
      document.getElementById("m-protocols").textContent = data.counts.protocols;

      const t = data.tower;
      const latestProgress = (t.Goal.Progress || []).slice(-1)[0] || "waiting for first admitted contribution";
      document.getElementById("endpoint").textContent = data.uptime + " - dynamic root " + data.dynamic_root;
      document.getElementById("objective").innerHTML =
        '<b>' + esc((t.Goal.Statements || [])[0] || "No shared objective declared yet") + '</b>'
        + '<span>Latest shared update: ' + esc(latestProgress) + '</span>';

      document.getElementById("protocols").innerHTML = (data.protocols || []).map(p => {
        const status = esc(p.status || "unknown");
        return '<div class="protocol ' + status + '">'
          + '<div class="protocol-head"><span>' + code(p.name) + '</span><span class="pill ' + status + '">' + status + '</span></div>'
          + '<p>' + esc(p.purpose || "") + '</p>'
          + '<p>' + esc(p.summary || "") + '</p>'
          + '<p>' + code(p.observed_type) + ' -> ' + code(p.resource) + ' - uses ' + esc(p.uses || 0) + '</p>'
          + '</div>';
      }).join("");

      document.getElementById("agents").innerHTML = data.appservers.map(a => {
        const state = esc(a.state || "unknown");
	        return '<div class="agent ' + state + '">'
	          + '<div class="agent-head"><span>' + esc(a.role) + ' - ' + code(a.principal) + '</span><span class="pill ' + state + '">' + state + '</span></div>'
	          + '<p>' + esc(a.last_summary || "waiting for first observation") + '</p>'
	          + '<p>' + esc(a.observations) + ' contributions admitted' + (a.last_seq ? ' - latest record #' + esc(a.last_seq) : '') + '</p>'
	          + (a.last_error ? '<p>' + esc(a.last_error) + '</p>' : "")
	          + '</div>';
	      }).join("");

      document.getElementById("tasks").innerHTML = data.tasks.map(t => {
        const state = esc(t.state || "unknown");
        const detail = t.assignee ? code(t.assignee) + " - " + esc(t.duration || "running") : "unassigned";
        const round = t.rounds ? "round " + esc(t.round || 0) + "/" + esc(t.rounds) : "";
        return '<div class="task ' + state + '">'
          + '<div class="task-head"><span>' + code(t.id) + '</span><span class="pill ' + state + '">' + state + '</span></div>'
          + '<p><strong>' + esc(t.title) + '</strong>' + (round ? ' - ' + round : '') + '</p>'
          + '<span class="worktext">' + esc(clip(t.command, 260)) + '</span>'
          + '<p>' + detail + (t.thread_id ? " - thread " + code(t.thread_id) : "") + '</p>'
          + (t.error ? '<p>' + esc(t.error) + '</p>' : "")
          + (t.output ? '<p class="pre">' + esc(clip(t.output, 720)) + '</p>' : "")
          + '</div>';
      }).join("");

      const visibleMessages = (data.messages || []).filter(m => m.kind !== "prompt" && m.kind !== "appserver");
      document.getElementById("messages").innerHTML = visibleMessages.slice(-24).reverse().map(m => {
        const label = m.kind === "final" ? "Agent report" : (m.kind === "activity" ? "Tool activity" : m.kind);
        return '<div class="message">'
          + '<div class="message-head"><strong>' + esc(label) + '</strong><span>' + code(m.principal) + ' / ' + code(m.task_id) + ' / round ' + esc(m.round || 0) + '</span></div>'
          + '<p>' + esc(clip(m.text, 1200)) + '</p>'
          + '</div>';
      }).join("") || '<div class="message"><p class="empty">waiting for real app-server messages</p></div>';

      const attention = [];
      (t.Inbox.Escalations || []).forEach(e => attention.push(esc(e.Domain) + ": " + esc(e.Reason)));
      (data.protocols || []).filter(p => p.source === "dynamic" && p.status === "waiting").forEach(p => attention.push(esc(p.name) + " is waiting for loopdef activation."));
      (data.protocols || []).filter(p => p.source === "dynamic" && p.status === "error").forEach(p => attention.push(esc(p.name) + " protocol error: " + esc(p.summary)));
      (data.tasks || []).filter(t => t.state === "failed").forEach(t => attention.push(esc(t.id) + " failed: " + esc(t.error || "unknown error")));
      (data.tasks || []).filter(t => t.state === "running" && t.rounds && t.round < t.rounds).slice(0, 3).forEach(t => {
        attention.push(esc(t.id) + " is waiting for later-round reactions from the team.");
      });
      document.getElementById("attention").innerHTML = attention.length
        ? attention.map(a => '<div class="attention-item">' + a + '</div>').join("")
        : '<div class="attention-item clear">No operator action required. The team is exchanging admitted observations normally.</div>';

      document.getElementById("events").innerHTML = (data.events || []).slice(-40).reverse().map(e => {
        const bad = e.error ? ' failed' : '';
        return '<div class="event' + bad + '">'
          + '<div class="event-head"><strong>seq ' + esc(e.seq) + '</strong><span>' + code(e.principal) + ' - ' + esc(e.type) + '</span></div>'
          + '<p>' + esc(e.summary || e.error || "observed") + '</p>'
          + '</div>';
      }).join("") || '<div class="event"><p class="empty">no observations yet</p></div>';
      const goalRows = [
	        ...(t.Goal.Statements || []).map(s => li("intent: " + esc(s))),
	        ...(t.Goal.Progress || []).slice(-8).map(s => li("progress: " + esc(s)))
      ];
      list("goal", goalRows, "no project intent");

      const fieldRows = [
	        ...(t.Field.Agents || []).map(a => li("agent: " + code(a.Principal) + " (" + esc(a.Kind) + ")")),
	        ...(t.Field.Assignments || []).map(a => li("assignment: " + esc(a.Scope) + " -> " + code(a.Assignee) + " (lease " + esc(a.TTL) + ")")),
	        li("escalations: " + esc(t.Field.Diagnostics || 0))
      ];
      list("field", fieldRows, "no field data");

	      const inboxRows = (t.Inbox.Escalations || []).map(e => li(esc(e.Domain) + ": " + code(e.Actor) + " [" + esc(e.Stage) + "] " + esc(e.Reason)));
      list("inbox", inboxRows, "inbox clear");

      const ledgerRows = (t.Ledger.Decisions || []).slice(-12).map(d => {
        const refs = (d.Refs || []).map(r => esc(r.Kind)).join(", ");
	        return li(code(d.DecisionID) + " by " + code(d.Actor) + " -> " + refs);
      });
      list("ledger", ledgerRows, "no decisions");
    }
    refresh();
    setInterval(refresh, 1000);
  </script>
</body>
</html>`))
