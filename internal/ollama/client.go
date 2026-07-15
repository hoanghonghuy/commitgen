package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/httpx"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// Config holds Ollama specific settings
type Config struct {
	BaseURL string // e.g. "http://localhost:11434" or "https://ollama.com"
	Model   string // e.g. "llama3" or "deepseek-v4-pro"
	APIKey  string // optional: API key for Ollama Cloud
}

// Client implements ai.Provider for Ollama
type Client struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

func New(cfg Config) *Client {
	baseURL := ResolveBaseURL(cfg.BaseURL, cfg.APIKey)
	return &Client{
		baseURL: baseURL,
		model:   cfg.Model,
		apiKey:  cfg.APIKey,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  options   `json:"options"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type options struct {
	Temperature float64 `json:"temperature"`
}

type chatResponse struct {
	Message message `json:"message"`
	Done    bool    `json:"done"`
}

func (c *Client) Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	return c.generate(ctx, msgs, temperature)
}

// endpoint returns the chat URL for the configured base URL.
func (c *Client) endpoint() string {
	return c.baseURL + "/api/chat"
}

// buildRequest assembles the chat request payload shared by the blocking and
// streaming code paths.
func (c *Client) buildRequest(msgs []vscodeprompt.VSCodeMessage, temperature float64, stream bool) chatRequest {
	return chatRequest{
		Model:    c.model,
		Messages: toOllamaMessages(msgs),
		Stream:   stream,
		Options:  options{Temperature: temperature},
	}
}

// toOllamaMessages converts VSCode messages to Ollama's chat format.
func toOllamaMessages(msgs []vscodeprompt.VSCodeMessage) []message {
	ollamaMsgs := make([]message, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		switch m.Role {
		case vscodeprompt.RoleAssistant:
			role = "assistant"
		case vscodeprompt.RoleSystem:
			role = "system"
		}
		var contentBuilder strings.Builder
		for _, part := range m.Content {
			contentBuilder.WriteString(part.Text)
		}
		ollamaMsgs = append(ollamaMsgs, message{Role: role, Content: contentBuilder.String()})
	}
	return ollamaMsgs
}

// GenerateStream streams the completion using Ollama's NDJSON stream, invoking
// onDelta for each chunk. It returns the full accumulated message.
func (c *Client) GenerateStream(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64, onDelta func(string)) (string, error) {
	payload, err := json.Marshal(c.buildRequest(msgs, temperature, true))
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.endpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", logger.LogError(err, "ollama: stream request failed", "url", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: API error (status %d): %s", resp.StatusCode, truncateOllamaErrorBody(body))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk chatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}
		if chunk.Message.Content != "" {
			full.WriteString(chunk.Message.Content)
			if onDelta != nil {
				onDelta(chunk.Message.Content)
			}
		}
		if chunk.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", logger.LogError(err, "ollama: stream read failed")
	}
	return full.String(), nil
}

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	reqBody := c.buildRequest(msgs, temperature, false)

	url := c.endpoint()
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}

	var chatResp chatResponse
	if err := httpx.DoJSONRequest(ctx, c.client, "POST", url, headers, reqBody, &chatResp); err != nil {
		return "", logger.LogError(fmt.Errorf("ollama: %w", err), "ollama: request failed", "url", url)
	}

	return chatResp.Message.Content, nil
}

func truncateOllamaErrorBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 500 {
		return s[:500] + "..."
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}
