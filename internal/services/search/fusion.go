package search

import "sort"

// FuseResults combines FTS and semantic results using Reciprocal Rank Fusion.
// k is the RRF constant (standard value: 60).
func FuseResults(ftsResults, semanticResults []SearchResult, k int) []SearchResult {
	if k <= 0 {
		k = 60
	}

	scores := make(map[string]float64)
	items := make(map[string]SearchResult)

	for rank, r := range ftsResults {
		scores[r.ID] += 1.0 / float64(k+rank+1)
		items[r.ID] = r
	}
	for rank, r := range semanticResults {
		scores[r.ID] += 1.0 / float64(k+rank+1)
		if _, exists := items[r.ID]; !exists {
			items[r.ID] = r
		}
	}

	merged := make([]SearchResult, 0, len(items))
	for id, item := range items {
		item.Score = scores[id]
		merged = append(merged, item)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	return merged
}
