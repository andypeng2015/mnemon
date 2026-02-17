package graph

import (
	"sort"

	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/search"
	"github.com/Grivn/mnemon/internal/store"
)

// Minimum token similarity to be considered a semantic candidate.
const minSemanticSimilarity = 0.10

// Maximum number of semantic candidates to return.
const maxSemanticCandidates = 5

// SemanticCandidate represents a potential semantic link for Claude to evaluate.
type SemanticCandidate struct {
	ID              string `json:"id"`
	Content         string `json:"content"`
	Category        string `json:"category"`
	TokenSimilarity float64 `json:"token_similarity"`
}

// FindSemanticCandidates returns insights that are potential semantic matches
// for the given insight, based on token overlap. These are candidates only —
// Claude evaluates and creates actual semantic edges via `mnemon link`.
func FindSemanticCandidates(db *store.DB, insight *model.Insight) []SemanticCandidate {
	all, err := db.GetAllActiveInsights()
	if err != nil || len(all) == 0 {
		return nil
	}

	type scored struct {
		insight    *model.Insight
		similarity float64
	}

	var candidates []scored
	for _, other := range all {
		if other.ID == insight.ID {
			continue
		}
		sim := search.ContentSimilarity(insight.Content, other.Content)
		if sim >= minSemanticSimilarity {
			candidates = append(candidates, scored{insight: other, similarity: sim})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})

	if len(candidates) > maxSemanticCandidates {
		candidates = candidates[:maxSemanticCandidates]
	}

	result := make([]SemanticCandidate, len(candidates))
	for i, c := range candidates {
		result[i] = SemanticCandidate{
			ID:              c.insight.ID,
			Content:         c.insight.Content,
			Category:        string(c.insight.Category),
			TokenSimilarity: c.similarity,
		}
	}
	return result
}
