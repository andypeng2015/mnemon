package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func testCommand() (*cobra.Command, *bytes.Buffer) {
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	return cmd, &output
}

func mustTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd, _ := testCommand()
	return cmd
}
