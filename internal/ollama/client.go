package ollama

import (
	"context"
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

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	// Convert VSCode messages to Ollama format
	ollamaMsgs := make([]message, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		switch m.Role {
		case vscodeprompt.RoleAssistant:
			role = "assistant"
		case vscodeprompt.RoleSystem:
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
