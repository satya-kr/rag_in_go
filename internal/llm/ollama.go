package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	BaseURL string
	Model   string
}

func NewClient() *Client {
	return &Client{
		BaseURL: "http://localhost:11434",
		Model:   "deepseek-r1:latest",
	}
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

func (c *Client) Generate(
	ctx context.Context,
	prompt string,
) (string, error) {

	requestBody := generateRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: false,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/api/generate",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"ollama returned status %d",
			resp.StatusCode,
		)
	}

	var result generateResponse

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", fmt.Errorf(
			"decode ollama response: %w",
			err,
		)
	}

	return result.Response, nil
}
