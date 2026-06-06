package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/server"
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	statusRoot        string
	statusProjectRoot string
	statusPrincipal   string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Agent Integration, Local Mnemon, and Remote Workspace status",
	RunE:  runProductStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusRoot, "root", ".", "repository root containing harness declarations")
	statusCmd.Flags().StringVar(&statusProjectRoot, "project-root", "", "project root for Agent Integration artifacts")
	statusCmd.Flags().StringVar(&statusPrincipal, "principal", "", "Agent Integration principal")
	statusCmd.GroupID = groupSpine
	rootCmd.AddCommand(statusCmd)
}

func runProductStatus(cmd *cobra.Command, args []string) error {
	root := filepath.Clean(statusRoot)
	projectRoot := statusProjectRoot
	if projectRoot == "" {
		projectRoot = root
	}
	projectRoot = filepath.Clean(projectRoot)

	if cfg, err := readLocalConfig(projectRoot); err == nil {
		principal := statusPrincipal
		if principal == "" {
			principal = cfg.Principal
		}
		if st, ok := localServiceStatus(projectRoot, cfg, principal); ok {
			printProductStatus(cmd, true, true, st.SyncPending, st.SyncSynced, st.SyncConflicts)
			return nil
		}
	}

	lines, err := app.New(root).SetupStatus(projectRoot, statusPrincipal)
	if err != nil {
		return err
	}
	for _, l := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), l)
	}
	counts := syncCounts(projectRoot)
	fmt.Fprintf(cmd.OutOrStdout(), "Sync: %d pending, %d synced, %d conflicts\n", counts.Pending, counts.Synced, counts.Conflicts)
	return nil
}

func localServiceStatus(projectRoot string, cfg localConfig, principal string) (server.ChannelStatus, bool) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(principal) == "" {
		return server.ChannelStatus{}, false
	}
	bindingFile := cfg.BindingFile
	if bindingFile == "" {
		bindingFile = server.DefaultBindingFile
	}
	loaded, err := server.LoadBindingFile(projectRoot, resolveProjectPath(projectRoot, bindingFile))
	if err != nil {
		return server.ChannelStatus{}, false
	}
	client := server.NewClient(cfg.Endpoint, contract.ActorID(principal))
	if tok := tokenForPrincipal(loaded.Tokens, contract.ActorID(principal)); tok != "" {
		client = server.NewClientWithToken(cfg.Endpoint, tok)
	}
	st, err := client.Status(contract.ActorID(principal))
	if err != nil {
		return server.ChannelStatus{}, false
	}
	return st, true
}

func printProductStatus(cmd *cobra.Command, installed, ready bool, pending, synced, conflicts int) {
	if installed {
		fmt.Fprintln(cmd.OutOrStdout(), "Agent Integration: installed")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Agent Integration: not installed")
	}
	if ready {
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: ready")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: not configured")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: not connected")
	fmt.Fprintf(cmd.OutOrStdout(), "Sync: %d pending, %d synced, %d conflicts\n", pending, synced, conflicts)
}

func tokenForPrincipal(tokens map[string]contract.ActorID, principal contract.ActorID) string {
	for tok, owner := range tokens {
		if owner == principal {
			return tok
		}
	}
	return ""
}

func syncCounts(projectRoot string) kernel.SyncCommitCounts {
	storePath := filepath.Join(projectRoot, server.DefaultStorePath)
	if _, err := os.Stat(storePath); err != nil {
		return kernel.SyncCommitCounts{}
	}
	store, err := kernel.OpenStore(storePath)
	if err != nil {
		return kernel.SyncCommitCounts{}
	}
	defer store.Close()
	counts, err := store.SyncCommitCounts()
	if err != nil {
		return kernel.SyncCommitCounts{}
	}
	return counts
}
