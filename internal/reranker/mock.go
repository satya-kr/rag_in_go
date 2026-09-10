package reranker

import (
	"context"
	"sort"
	"strings"
)

type MockReranker struct{}

func NewMockReranker() *MockReranker {
	return &MockReranker{}
}

// Rerank scores documents by counting query word matches (for local testing).
func (r *MockReranker) Rerank(
	ctx context.Context,
	query string,
	documents []Document,
	topK int,
) ([]Document, error) {

	queryWords := strings.Fields(strings.ToLower(query))

	for i := range documents {
		content := strings.ToLower(documents[i].Content)

		var score float64

		for _, word := range queryWords {
			if strings.Contains(content, word) {
				score++
			}
		}

		documents[i].RerankScore = score
	}

	sort.Slice(
		documents,
		func(i, j int) bool {
			return documents[i].RerankScore > documents[j].RerankScore
		},
	)

	if topK > len(documents) {
		topK = len(documents)
	}

	return documents[:topK], nil
}
