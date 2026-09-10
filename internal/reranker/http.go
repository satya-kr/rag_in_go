package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

type HTTPReranker struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPReranker(baseURL string) *HTTPReranker {
	return &HTTPReranker{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type rerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type rerankResult struct {
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

type rerankResponse struct {
	Results []rerankResult `json:"results"`
}

func (r *HTTPReranker) Rerank(
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

	requestDocuments := make([]string, len(documents))

	for i, doc := range documents {
		requestDocuments[i] = doc.Content
	}

	payload := rerankRequest{
		Query:     query,
		Documents: requestDocuments,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		r.baseURL+"/rerank",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create rerank request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call reranker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf(
			"reranker returned HTTP status %d",
			resp.StatusCode,
		)
	}

	var result rerankResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}

	for _, item := range result.Results {

		if item.Index < 0 || item.Index >= len(documents) {
			continue
		}

		documents[item.Index].RerankScore = item.Score
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
