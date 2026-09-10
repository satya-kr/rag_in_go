package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"
)

type CohereReranker struct {
	apiKey     string
	httpClient *http.Client
}

func NewCohereReranker(apiKey string) *CohereReranker {
	return &CohereReranker{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type cohereRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type cohereResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type cohereResponse struct {
	Results []cohereResult `json:"results"`
}

func (r *CohereReranker) Rerank(
	ctx context.Context,
	query string,
	documents []Document,
	topK int,
) ([]Document, error) {

	if len(documents) == 0 {
		return []Document{}, nil
	}

	if topK <= 0 {
		return []Document{}, nil
	}

	if topK > len(documents) {
		topK = len(documents)
	}

	// Convert our documents into Cohere's expected format.
	requestDocuments := make([]string, len(documents))

	for i, doc := range documents {
		requestDocuments[i] = doc.Content
	}

	payload := cohereRequest{
		Model:     "rerank-v4.0-fast",
		Query:     query,
		Documents: requestDocuments,
		TopN:      topK,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal cohere request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.cohere.com/v2/rerank",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create cohere request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call cohere reranker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"cohere reranker returned HTTP status %d",
			resp.StatusCode,
		)
	}

	var result cohereResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode cohere response: %w", err)
	}

	// Map Cohere's result index back to our original documents.
	for _, item := range result.Results {

		if item.Index < 0 || item.Index >= len(documents) {
			continue
		}

		documents[item.Index].RerankScore = item.RelevanceScore
	}

	// Sort by the ML reranking score.
	sort.Slice(
		documents,
		func(i, j int) bool {
			return documents[i].RerankScore > documents[j].RerankScore
		},
	)

	return documents[:topK], nil
}

// NewCohereRerankerFromEnv creates the reranker using
// the COHERE_API_KEY environment variable.
func NewCohereRerankerFromEnv() (*CohereReranker, error) {

	apiKey := os.Getenv("COHERE_API_KEY")

	if apiKey == "" {
		return nil, fmt.Errorf("COHERE_API_KEY environment variable is not set")
	}

	return NewCohereReranker(apiKey), nil
}
