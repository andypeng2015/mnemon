package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/search"
	"github.com/spf13/cobra"
)

var diffLimit int

var diffCmd = &cobra.Command{
	Use:   "diff [new content]",
	Short: "Check for duplicates or conflicts",
	Long:  "Compare new content against existing insights to detect duplicates, conflicts, or updates.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		newFact := strings.Join(args, " ")

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		all, err := db.GetAllActiveInsights()
		if err != nil {
			return fmt.Errorf("get insights: %w", err)
		}

		// Optionally compute embedding for the new content
		opts := search.DiffOptions{Limit: diffLimit}
		ec := embed.NewClient()
		if ec.Available() {
			if vec, err := ec.Embed(newFact); err == nil {
				opts.NewEmbedding = vec
			}
			// Load existing embeddings
			embeds, err := db.GetAllEmbeddings()
			if err == nil {
				opts.ExistingEmbed = make([]search.EmbeddedItem, 0, len(embeds))
				for _, e := range embeds {
					if v := embed.DeserializeVector(e.Embedding); v != nil {
						opts.ExistingEmbed = append(opts.ExistingEmbed, search.EmbeddedItem{
							ID:        e.ID,
							Embedding: v,
						})
					}
				}
			}
		}

		result := search.Diff(all, newFact, opts)

		db.LogOp("diff", "", fmt.Sprintf("suggestion=%s content=%s", result.Suggestion, newFact))

		output := map[string]interface{}{
			"new_fact":   newFact,
			"suggestion": result.Suggestion,
			"similar":    result.Matches,
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

func init() {
	diffCmd.Flags().IntVar(&diffLimit, "limit", 5, "max similar results to compare")
	rootCmd.AddCommand(diffCmd)
}
