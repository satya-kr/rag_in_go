package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

var textModel = "nomic-embed-text"

type Embedding []float32

type Client struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewClient() *Client {
	return &Client{
		BaseURL: "http://localhost:11434",
		Model:   textModel,
		Client:  &http.Client{},
	}
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (c *Client) CreateEmbedding(
	ctx context.Context,
	text string,
) (Embedding, error) {

	reqBody := embeddingRequest{
		Model: c.Model,
		Input: text,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/api/embed",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"ollama returned status %d",
			resp.StatusCode,
		)
	}

	var result embeddingResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf(
			"decode embedding response: %w",
			err,
		)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned no embedding")
	}

	return Embedding(result.Embeddings[0]), nil
}
