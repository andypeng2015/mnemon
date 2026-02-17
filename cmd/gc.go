package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	gcThreshold float64
	gcLimit     int
	gcKeepID    string
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Review memory retention and suggest cleanup",
	Long: `Garbage collection for memory insights. Two modes:

Suggest mode (default):
  mnemon gc [--threshold 0.4] [--limit 20]
  Outputs low-retention insights for Claude to review.

Keep mode:
  mnemon gc --keep <id>
  Boosts an insight's retention score (access_count +3, refreshes timestamp).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// Keep mode: boost retention for a specific insight
		if gcKeepID != "" {
			ins, err := db.GetInsightByID(gcKeepID)
			if err != nil || ins == nil {
				return fmt.Errorf("insight %s not found", gcKeepID)
			}
			if err := db.BoostRetention(gcKeepID); err != nil {
				return fmt.Errorf("boost retention: %w", err)
			}
			db.LogOp("gc_keep", gcKeepID, ins.Content)

			output := map[string]interface{}{
				"status":     "retained",
				"id":         gcKeepID,
				"content":    ins.Content,
				"new_access": ins.AccessCount + 3,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(output)
		}

		// Suggest mode: find low-retention candidates
		candidates, total, err := db.GetRetentionCandidates(gcThreshold, gcLimit)
		if err != nil {
			return fmt.Errorf("get retention candidates: %w", err)
		}

		db.LogOp("gc", "", fmt.Sprintf("threshold=%.2f found=%d total=%d", gcThreshold, len(candidates), total))

		output := map[string]interface{}{
			"total_insights":   total,
			"threshold":        gcThreshold,
			"candidates_found": len(candidates),
			"candidates":       candidates,
			"actions": map[string]string{
				"purge": "mnemon forget <id>",
				"keep":  "mnemon gc --keep <id>",
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

func init() {
	gcCmd.Flags().Float64Var(&gcThreshold, "threshold", 0.4, "retention score threshold (0.0-1.0)")
	gcCmd.Flags().IntVar(&gcLimit, "limit", 20, "max candidates to return")
	gcCmd.Flags().StringVar(&gcKeepID, "keep", "", "boost retention for this insight ID")
	rootCmd.AddCommand(gcCmd)
}
