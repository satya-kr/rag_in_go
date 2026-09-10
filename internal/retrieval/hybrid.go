package retrieval

import (
	"go_rag/internal/documentchunk"
	"sort"
)

type HybridResult struct {
	Chunk       documentchunk.Chunk
	VectorRank  int
	KeywordRank int
	RRFScore    float64
}

func RRF(
	vectorResults []documentchunk.Chunk,
	keywordResults []documentchunk.Chunk,
	k int,
	topK int,
) []HybridResult {

	results := make(map[int64]*HybridResult)

	// --------------------------------
	// VECTOR RESULTS
	// --------------------------------

	for rank, chunk := range vectorResults {

		position := rank + 1

		result, exists := results[chunk.ID]

		if !exists {
			result = &HybridResult{
				Chunk: chunk,
			}

			results[chunk.ID] = result
		}

		result.VectorRank = position

		result.RRFScore += 1.0 / float64(k+position)
	}

	// --------------------------------
	// KEYWORD RESULTS
	// --------------------------------

	for rank, chunk := range keywordResults {

		position := rank + 1

		result, exists := results[chunk.ID]

		if !exists {
			result = &HybridResult{
				Chunk: chunk,
			}

			results[chunk.ID] = result
		}

		result.KeywordRank = position

		result.RRFScore += 1.0 / float64(k+position)
	}

	// --------------------------------
	// CONVERT MAP → SLICE
	// --------------------------------

	finalResults := make([]HybridResult, 0, len(results))

	for _, result := range results {
		finalResults = append(finalResults, *result)
	}

	// --------------------------------
	// SORT BY RRF SCORE
	// --------------------------------

	sort.Slice(
		finalResults,
		func(i, j int) bool {
			return finalResults[i].RRFScore >
				finalResults[j].RRFScore
		},
	)

	// return finalResults

	if topK > len(results) {
		topK = len(results)
	}

	return finalResults[:topK]
}
