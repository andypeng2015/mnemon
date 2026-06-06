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

func init() {
	loopCmd.PersistentFlags().StringVar(&loopRoot, "root", ".", "repository root containing harness declarations")
	loopCmd.AddCommand(loopValidateCmd)
	loopCmd.GroupID = groupSpine
	rootCmd.AddCommand(loopCmd)
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
