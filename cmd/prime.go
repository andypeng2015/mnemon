package cmd

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/config"
	"github.com/mnemon-dev/mnemon/internal/setup"
	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Output session-start guidance for hook injection",
	Long:  "Compose behavioral guidance and memory status for LLM context injection via SessionStart hook.",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("target")
		showStatus, _ := cmd.Flags().GetBool("status")

		cfg, err := config.Load(dataDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		var stats *store.InsightStats
		if showStatus {
			db, err := openDB()
			if err == nil {
				stats, _ = db.GetStats()
				db.Close()
			}
		}

		fmt.Print(composePrime(cfg, target, stats))
		return nil
	},
}

func init() {
	primeCmd.Flags().String("target", "claude-code", "target platform (claude-code, openclaw)")
	primeCmd.Flags().Bool("status", true, "include memory status line")
	rootCmd.AddCommand(primeCmd)
}

func composePrime(cfg *config.Config, target string, stats *store.InsightStats) string {
	var b strings.Builder

	if stats != nil {
		b.WriteString(fmt.Sprintf("[mnemon] Memory active (%d insights, %d edges).\n\n",
			stats.Total, stats.EdgeCount))
	}

	for _, sec := range cfg.Prime.Sections {
		switch sec {
		case "recall":
			b.WriteString(setup.RecallGuidance)
			b.WriteString("\n")
		case "remember":
			if cfg.Prime.RememberText != "" {
				b.WriteString(cfg.Prime.RememberText)
			} else {
				b.WriteString(setup.RememberGuidance)
			}
			b.WriteString("\n")
		case "delegation":
			if target == "openclaw" {
				b.WriteString(setup.OpenClawDelegation)
			} else {
				b.WriteString(setup.ClaudeDelegationWithModel(cfg.Prime.DelegationModel))
			}
			b.WriteString("\n")
		}
	}

	if cfg.Prime.Custom != "" {
		b.WriteString(cfg.Prime.Custom)
		b.WriteString("\n")
	}

	return b.String()
}
