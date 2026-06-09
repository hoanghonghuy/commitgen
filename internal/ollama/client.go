package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/httpx"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// Config holds Ollama specific settings
type Config struct {
	BaseURL string // e.g. "http://localhost:11434"
	Model   string // e.g. "llama3"
}

// Client implements ai.Provider for Ollama
type Client struct {
	baseURL string
	model   string
	client  *http.Client
}

func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Client{
		baseURL: baseURL,
		model:   cfg.Model,
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
	reqBody := chatRequest{
		Model:    c.model,
		Messages: toOllamaMessages(msgs),
		Stream:   true,
		Options:  options{Temperature: temperature},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", logger.LogError(err, "ollama: stream request failed", "url", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: API error (status %d)", resp.StatusCode)
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
	reqBody := chatRequest{
		Model:    c.model,
		Messages: toOllamaMessages(msgs),
		Stream:   false,
		Options: options{
			Temperature: temperature,
		},
	}

	url := fmt.Sprintf("%s/api/chat", c.baseURL)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	var chatResp chatResponse
	if err := httpx.DoJSONRequest(ctx, c.client, "POST", url, headers, reqBody, &chatResp); err != nil {
		return "", logger.LogError(err, "ollama: request failed", "url", url)
	}

	return chatResp.Message.Content, nil
}
