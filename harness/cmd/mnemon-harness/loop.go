package main

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	loopRoot string
)

var loopCmd = &cobra.Command{
	Use:    "loop",
	Short:  "Validate harness declarations",
	Hidden: true,
}

var loopValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate harness loop, host, and binding declarations",
	RunE:  runLoopValidate,
}

var loopAddCmd = &cobra.Command{
	Use:   "add <dir>",
	Short: "Register an external capability package from a directory",
	Args:  cobra.ExactArgs(1),
	RunE:  runLoopAdd,
}

func init() {
	loopCmd.PersistentFlags().StringVar(&loopRoot, "root", ".", "repository root containing harness declarations")
	loopCmd.AddCommand(loopValidateCmd, loopAddCmd)
	loopCmd.GroupID = groupSpine
	rootCmd.AddCommand(loopCmd)
}

func runLoopAdd(cmd *cobra.Command, args []string) error {
	name, err := app.New(loopRoot).LoopAdd(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "added loop %q under .mnemon/loops/%s; enable it with: mnemon-harness setup --host HOST --loop %s\n", name, name, name)
	return nil
}

func runLoopValidate(cmd *cobra.Command, args []string) error {
	lines, err := app.New(loopRoot).LoopValidate()
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}
