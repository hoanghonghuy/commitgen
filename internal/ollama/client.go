package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
		client:  &http.Client{},
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

func (c *Client) GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	// Convert VSCode messages to Ollama format
	ollamaMsgs := make([]message, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		switch m.Role {
		case 2: // Assistant
			role = "assistant"
		case 3: // System
			role = "system"
		}

		// Concatenate content parts
		var contentBuilder strings.Builder
		for _, part := range m.Content {
			contentBuilder.WriteString(part.Text)
		}
		ollamaMsgs = append(ollamaMsgs, message{
			Role:    role,
			Content: contentBuilder.String(),
		})
	}

	reqBody := chatRequest{
		Model:    c.model,
		Messages: ollamaMsgs,
		Stream:   false,
		Options: options{
			Temperature: temperature,
		},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return chatResp.Message.Content, nil
}
