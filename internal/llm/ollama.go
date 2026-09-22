package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type Client struct {
	BaseURL    string
	Model      string
	httpClient *http.Client
}

func NewClient() *Client {
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "deepseek-r1:latest"
	}
	return &Client{
		BaseURL:    "http://localhost:11434",
		Model:      model,
		httpClient: &http.Client{},
	}
}

type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

// generateStreamChunk is one line from Ollama's NDJSON stream.
type generateStreamChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate calls Ollama with stream:true and reads NDJSON chunks until done.
// For deepseek-r1 models, think:false is passed to suppress the chain-of-thought
// block at the Ollama level. The output is also post-processed to strip any
// <think>...</think> that leaks through regardless.
func (c *Client) Generate(
	ctx context.Context,
	prompt string,
) (string, error) {

	req := generateRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: true,
	}

	// deepseek-r1 supports think:false to disable chain-of-thought output.
	// Without this the model emits a massive <think> block before every answer.
	if strings.Contains(strings.ToLower(c.Model), "deepseek") {
		req.Options = map[string]any{"think": false}
		log.Printf("[LLM]  deepseek model detected — setting think:false")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/api/generate",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	// Read NDJSON stream — one JSON object per line until done:true.
	var sb strings.Builder
	decoder := json.NewDecoder(resp.Body)
	tokenCount := 0

	for {
		var chunk generateStreamChunk
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("decode ollama stream: %w", err)
		}

		sb.WriteString(chunk.Response)
		tokenCount++

		// Log every 50 tokens so the terminal shows the model is alive
		if tokenCount%50 == 0 {
			log.Printf("[LLM]  streaming...  tokens=%d", tokenCount)
		}

		if chunk.Done {
			log.Printf("[LLM]  stream complete  tokens=%d  raw_chars=%d",
				tokenCount, sb.Len())
			break
		}
	}

	raw := sb.String()
	clean := stripThinkBlocks(raw)

	if len(clean) < len(raw) {
		log.Printf("[LLM]  stripped think blocks  raw=%d  clean=%d chars",
			len(raw), len(clean))
	}

	if strings.TrimSpace(clean) == "" {
		// Model produced only a think block and no answer — return a safe fallback
		log.Printf("[LLM]  WARNING: answer was empty after stripping think blocks")
		return "I don't know based on the provided documents.", nil
	}

	return clean, nil
}

// thinkRe matches <think>...</think> blocks including whitespace variants.
// (?i) = case-insensitive, (?s) = dot matches newline.
var thinkRe = regexp.MustCompile(`(?is)<think>.*?</think>`)

// stripThinkBlocks removes all <think>...</think> reasoning blocks.
// Also handles unclosed tags by stripping from <think> to end of string.
func stripThinkBlocks(s string) string {
	// Remove closed blocks first
	s = thinkRe.ReplaceAllString(s, "")

	// Remove any unclosed <think> block (model stopped mid-thought)
	if idx := strings.Index(strings.ToLower(s), "<think>"); idx != -1 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}
