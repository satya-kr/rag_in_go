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

func isOpenAIModel(model string) bool {
	return strings.HasPrefix(model, "gpt-")
}

// ── Ollama types ──────────────────────────────────────────────────────────────

type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

type generateStreamChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// ── OpenAI types ──────────────────────────────────────────────────────────────

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ── Generate ──────────────────────────────────────────────────────────────────

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	if isOpenAIModel(c.Model) {
		return c.generateOpenAI(ctx, prompt)
	}
	return c.generateOllama(ctx, prompt)
}

func (c *Client) generateOpenAI(ctx context.Context, prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY not set")
	}

	reqBody := openAIRequest{
		Model:    c.Model,
		Messages: []openAIMessage{{Role: "user", Content: prompt}},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	log.Printf("[LLM]  calling OpenAI model=%s", c.Model)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("openai error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	log.Printf("[LLM]  OpenAI done  answer_chars=%d", len(result.Choices[0].Message.Content))
	return result.Choices[0].Message.Content, nil
}

func (c *Client) generateOllama(ctx context.Context, prompt string) (string, error) {
	req := generateRequest{
		Model:  c.Model,
		Prompt: prompt,
		Stream: true,
	}

	if strings.Contains(strings.ToLower(c.Model), "deepseek") {
		req.Options = map[string]any{"think": false}
		log.Printf("[LLM]  deepseek model detected — setting think:false")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/generate", bytes.NewBuffer(body))
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

		if tokenCount%50 == 0 {
			log.Printf("[LLM]  streaming...  tokens=%d", tokenCount)
		}

		if chunk.Done {
			log.Printf("[LLM]  stream complete  tokens=%d  raw_chars=%d", tokenCount, sb.Len())
			break
		}
	}

	raw := sb.String()
	clean := stripThinkBlocks(raw)

	if len(clean) < len(raw) {
		log.Printf("[LLM]  stripped think blocks  raw=%d  clean=%d chars", len(raw), len(clean))
	}

	if strings.TrimSpace(clean) == "" {
		log.Printf("[LLM]  WARNING: answer was empty after stripping think blocks")
		return "I don't know based on the provided documents.", nil
	}

	return clean, nil
}

var thinkRe = regexp.MustCompile(`(?is)<think>.*?</think>`)

func stripThinkBlocks(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	if idx := strings.Index(strings.ToLower(s), "<think>"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
