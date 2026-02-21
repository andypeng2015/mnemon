package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var (
	relEdgeType string
	relDepth    int
)

var relatedCmd = &cobra.Command{
	Use:   "related [id]",
	Short: "Find related insights via graph traversal",
	Long:  "BFS traversal from a given insight, optionally filtered by edge type.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		startID := args[0]

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// Verify start node exists
		start, err := db.GetInsightByID(startID)
		if err != nil {
			return fmt.Errorf("insight not found: %w", err)
		}

		var edgeFilter model.EdgeType
		if relEdgeType != "" {
			et := model.EdgeType(relEdgeType)
			if !model.ValidEdgeTypes[et] {
				return fmt.Errorf("invalid edge type %q; valid: temporal, semantic, causal, entity", relEdgeType)
			}
			edgeFilter = et
		}

		// BFS traversal
		related := bfsTraverse(db, start.ID, edgeFilter, relDepth)

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(related)
	},
}

type relatedResult struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Category   string `json:"category"`
	Importance int    `json:"importance"`
	Depth      int    `json:"depth"`
	EdgeType   string `json:"via_edge_type,omitempty"`
}

func bfsTraverse(db *store.DB, startID string, edgeFilter model.EdgeType, maxDepth int) []relatedResult {
	// Pre-load all insights and edges for in-memory BFS (avoids N+1 queries).
	allInsights, err := db.GetAllActiveInsights()
	if err != nil {
		return nil
	}
	insightMap := make(map[string]*model.Insight, len(allInsights))
	for _, ins := range allInsights {
		insightMap[ins.ID] = ins
	}

	allEdges, err := db.GetAllEdges()
	if err != nil {
		return nil
	}
	edgeAdj := make(map[string][]*model.Edge)
	for _, e := range allEdges {
		edgeAdj[e.SourceID] = append(edgeAdj[e.SourceID], e)
		if e.SourceID != e.TargetID {
			edgeAdj[e.TargetID] = append(edgeAdj[e.TargetID], e)
		}
	}

	type queueItem struct {
		id       string
		depth    int
		edgeType string
	}

	visited := map[string]bool{startID: true}
	queue := []queueItem{{id: startID, depth: 0}}
	results := make([]relatedResult, 0)

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth > maxDepth {
			continue
		}

		// Add to results (skip the start node)
		if item.id != startID {
			insight := insightMap[item.id]
			if insight == nil {
				continue
			}
			results = append(results, relatedResult{
				ID:         insight.ID,
				Content:    insight.Content,
				Category:   string(insight.Category),
				Importance: insight.Importance,
				Depth:      item.depth,
				EdgeType:   item.edgeType,
			})
		}

		if item.depth >= maxDepth {
			continue
		}

		// Get edges from pre-loaded adjacency
		edges := edgeAdj[item.id]
		for _, e := range edges {
			if edgeFilter != "" && e.EdgeType != edgeFilter {
				continue
			}
			neighborID := e.TargetID
			if neighborID == item.id {
				neighborID = e.SourceID
			}
			if !visited[neighborID] {
				visited[neighborID] = true
				queue = append(queue, queueItem{
					id:       neighborID,
					depth:    item.depth + 1,
					edgeType: string(e.EdgeType),
				})
			}
		}
	}
	return results
}

func init() {
	relatedCmd.Flags().StringVar(&relEdgeType, "edge", "", "filter by edge type (temporal|semantic|causal|entity)")
	relatedCmd.Flags().IntVar(&relDepth, "depth", 2, "max traversal depth")
	rootCmd.AddCommand(relatedCmd)
}
