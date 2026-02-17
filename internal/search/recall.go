package search

import (
	"sort"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// Maximum traversal depth from each anchor point.
const maxTraversalDepth = 2

// RecallResult represents a recalled insight with its relevance score.
type RecallResult struct {
	Insight *model.Insight `json:"insight"`
	Score   float64        `json:"score"`
	Intent  Intent         `json:"intent"`
	Via     string         `json:"via,omitempty"` // how it was found
}

// IntentAwareRecall performs intent-aware retrieval:
// 1. Detect query intent
// 2. Keyword search (+ optional vector search) to find anchor points
// 3. Multi-level BFS from anchors with intent-weighted score decay
// 4. Merge and rank results
//
// queryVec is optional — when non-nil, vector search is fused with keyword
// search for anchor selection using Reciprocal Rank Fusion (RRF).
func IntentAwareRecall(db *store.DB, query string, queryVec []float64, limit int) ([]RecallResult, error) {
	intent := DetectIntent(query)
	weights := GetWeights(intent)

	// Step 1: Get all active insights for keyword search
	all, err := db.GetAllActiveInsights()
	if err != nil {
		return nil, err
	}

	// Step 2: Keyword search for anchors
	keywordAnchors := KeywordSearch(all, query, 5)

	// Build unified anchor list — keyword only, or RRF fusion with vectors
	type anchor struct {
		insight *model.Insight
		score   float64
		via     string
	}
	anchorMap := make(map[string]*anchor)

	// RRF constant (standard value from literature)
	const rrfK = 60

	// Add keyword anchors with RRF rank scores
	for rank, a := range keywordAnchors {
		anchorMap[a.Insight.ID] = &anchor{
			insight: a.Insight,
			score:   1.0 / float64(rrfK+rank+1),
			via:     "keyword",
		}
	}

	// If we have a query vector, do vector search and fuse via RRF
	if queryVec != nil {
		vectorHits := vectorSearch(db, queryVec, 5)
		for rank, vh := range vectorHits {
			rrfScore := 1.0 / float64(rrfK+rank+1)
			if existing, ok := anchorMap[vh.id]; ok {
				existing.score += rrfScore // fuse scores
				existing.via = "hybrid"
			} else {
				ins, err := db.GetInsightByID(vh.id)
				if err != nil || ins == nil {
					continue
				}
				anchorMap[vh.id] = &anchor{
					insight: ins,
					score:   rrfScore,
					via:     "vector",
				}
			}
		}
	}

	// Normalize anchor scores to [0, 1] range
	var maxAnchorScore float64
	for _, a := range anchorMap {
		if a.score > maxAnchorScore {
			maxAnchorScore = a.score
		}
	}
	if maxAnchorScore > 0 {
		for _, a := range anchorMap {
			a.score /= maxAnchorScore
		}
	}

	// Build score map: id -> best score found so far
	scoreMap := make(map[string]float64)
	viaMap := make(map[string]string)
	insightMap := make(map[string]*model.Insight)

	for id, a := range anchorMap {
		scoreMap[id] = a.score
		viaMap[id] = a.via
		insightMap[id] = a.insight
	}

	// Step 3: Multi-level BFS from each anchor
	type bfsItem struct {
		id    string
		score float64 // accumulated score arriving at this node
		depth int
	}

	for id, a := range anchorMap {
		queue := []bfsItem{{id: id, score: a.score, depth: 0}}
		visited := map[string]bool{id: true}

		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]

			if cur.depth >= maxTraversalDepth {
				continue
			}

			edges, err := db.GetEdgesByNode(cur.id)
			if err != nil {
				continue
			}

			for _, e := range edges {
				neighborID := e.TargetID
				if neighborID == cur.id {
					neighborID = e.SourceID
				}

				// Score decays: parent_score * intent_weight * edge_weight
				edgeWeight := weights[e.EdgeType]
				neighborScore := cur.score * edgeWeight * e.Weight

				// Boost with vector similarity if available
				if queryVec != nil {
					if blob, err := db.GetEmbedding(neighborID); err == nil && len(blob) > 0 {
						nVec := embed.DeserializeVector(blob)
						cosSim := embed.CosineSimilarity(queryVec, nVec)
						if cosSim > 0 {
							neighborScore += cosSim * 0.3 // λ₂ semantic boost
						}
					}
				}

				if existing, ok := scoreMap[neighborID]; !ok || neighborScore > existing {
					scoreMap[neighborID] = neighborScore
					viaMap[neighborID] = string(e.EdgeType)
					if _, loaded := insightMap[neighborID]; !loaded {
						ins, err := db.GetInsightByID(neighborID)
						if err == nil && ins != nil {
							insightMap[neighborID] = ins
						}
					}
				}

				if !visited[neighborID] {
					visited[neighborID] = true
					queue = append(queue, bfsItem{
						id:    neighborID,
						score: neighborScore,
						depth: cur.depth + 1,
					})
				}
			}
		}
	}

	// Step 4: Collect and sort results
	results := make([]RecallResult, 0, len(scoreMap))
	for id, score := range scoreMap {
		ins, ok := insightMap[id]
		if !ok {
			continue
		}
		results = append(results, RecallResult{
			Insight: ins,
			Score:   score,
			Intent:  intent,
			Via:     viaMap[id],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Insight.Importance > results[j].Insight.Importance
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// vectorHit is a vector search result.
type vectorHit struct {
	id         string
	similarity float64
}

// vectorSearch performs brute-force cosine similarity search over all embedded insights.
func vectorSearch(db *store.DB, queryVec []float64, limit int) []vectorHit {
	embedded, err := db.GetAllEmbeddings()
	if err != nil || len(embedded) == 0 {
		return nil
	}

	var hits []vectorHit
	for _, e := range embedded {
		vec := embed.DeserializeVector(e.Embedding)
		if vec == nil {
			continue
		}
		sim := embed.CosineSimilarity(queryVec, vec)
		if sim > 0.1 { // minimum similarity threshold
			hits = append(hits, vectorHit{id: e.ID, similarity: sim})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].similarity > hits[j].similarity
	})

	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
