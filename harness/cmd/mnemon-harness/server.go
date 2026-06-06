package main

import (
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/core/server"
	"github.com/spf13/cobra"
)

var (
	serverAddr         string
	serverStorePath    string
	serverBindingsPath string
)

// serverCmd + demoCmd fold the former standalone mnemon-control binary into the one harness
// binary (D2). Both reach the engine only through the channel package (server.ServerAPI /
// server.RunDemo), never kernel/reconcile directly (the P2.3 boundary, enforced by ringguard).

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the core control-plane channel (observe/pull) over httpapi",
	Long:  "Boot a ControlServer over a persistent kernel store and serve the channel (ServerAPI: observe via Ingest, pull via PullProjection) over httpapi until interrupted.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// When the operator did not pass an explicit --store, discover the project's canonical store by
		// walking up from the CWD for the .mnemon marker, so the server lands on the SAME store the
		// lifecycle/app apply surface uses regardless of which subdirectory it is booted from (no CWD
		// store split). An explicit --store is honored verbatim (OpenRuntime absolutizes it).
		root := server.DiscoverProjectRoot()
		storePath := serverStorePath
		if !cmd.Flags().Changed("store") {
			storePath = filepath.Join(root, server.DefaultStorePath)
		}
		// With --channel-bindings, the server enforces the binding manifest (BindingSet authorizer +
		// scopes + token auth). Without it, a bare channel endpoint (trusted-header auth).
		if serverBindingsPath != "" {
			bindingsPath := serverBindingsPath
			if !filepath.IsAbs(bindingsPath) {
				bindingsPath = filepath.Join(root, bindingsPath)
			}
			loaded, err := server.LoadBindingFile(root, bindingsPath)
			if err != nil {
				return err
			}
			return server.RunHTTPServerWithBindings(cmd.Context(), serverAddr, storePath, loaded, cmd.OutOrStdout())
		}
		return server.RunHTTPServer(cmd.Context(), serverAddr, storePath, cmd.OutOrStdout())
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run the self-checking full control-plane demo (exits 0 iff every link holds)",
	Long:  "Boot a ControlServer whose rule seat holds a real wazero WASM rule and drive two edges through the whole governed chain (deny/propose, CAS, conflict, scoped projection, job lane, receipt, tampered-readback, masked replay). Exits 0 iff every link holds.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.RunDemo(cmd.OutOrStdout())
	},
}

func init() {
	serverCmd.Flags().StringVar(&serverAddr, "addr", "127.0.0.1:8787", "listen address")
	serverCmd.Flags().StringVar(&serverStorePath, "store", server.DefaultStorePath, "kernel store path")
	serverCmd.Flags().StringVar(&serverBindingsPath, "channel-bindings", "", "channel binding manifest (enforces bindings + token auth); bare channel when unset")
	serverCmd.GroupID = groupSpine
	demoCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(serverCmd, demoCmd)
}
