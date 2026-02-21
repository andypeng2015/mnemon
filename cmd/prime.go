package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Output session-start guidance for hook injection",
	Long:  "Output memory status and guide.md content for LLM context injection.",
	RunE: func(cmd *cobra.Command, args []string) error {
		showStatus, _ := cmd.Flags().GetBool("status")

		var stats *store.InsightStats
		if showStatus {
			db, err := openDB()
			if err == nil {
				stats, _ = db.GetStats()
				db.Close()
			}
		}

		fmt.Print(composePrime(stats))
		return nil
	},
}

func init() {
	primeCmd.Flags().Bool("status", true, "include memory status line")
	rootCmd.AddCommand(primeCmd)
}

func composePrime(stats *store.InsightStats) string {
	var b strings.Builder

	if stats != nil {
		b.WriteString(fmt.Sprintf("[mnemon] Memory active (%d insights, %d edges).\n",
			stats.Total, stats.EdgeCount))
	} else {
		b.WriteString("[mnemon] Memory active\n")
	}

	home, err := os.UserHomeDir()
	if err == nil {
		guidePath := filepath.Join(home, ".mnemon", "prompt", "guide.md")
		if data, err := os.ReadFile(guidePath); err == nil {
			guide := strings.TrimSpace(string(data))
			if guide != "" {
				b.WriteString("\n")
				b.WriteString(guide)
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}
