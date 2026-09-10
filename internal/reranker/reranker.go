package reranker

import "context"

type Document struct {
	ID             int64
	Content        string
	RetrievalScore float64
	RerankScore    float64
}

type Reranker interface {
	Rerank(
		ctx context.Context,
		query string,
		documents []Document,
		topK int,
	) ([]Document, error)
}
