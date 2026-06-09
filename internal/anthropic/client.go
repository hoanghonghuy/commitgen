package anthropic

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

type Config struct {
	APIKey string
	Model  string
}

const anthropicMaxTokens = 4096

const defaultAnthropicURL = "https://api.anthropic.com/v1/messages"

type Client struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: defaultAnthropicURL,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type messageRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	System      string    `json:"system,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messageResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (c *Client) Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	return c.generate(ctx, msgs, temperature)
}

// buildRequest converts VSCode messages into Anthropic's request format. The
// system prompt is top-level; user/assistant messages alternate in Messages.
func (c *Client) buildRequest(msgs []vscodeprompt.VSCodeMessage, temperature float64, stream bool) messageRequest {
	var systemPrompt string
	var anthropicMsgs []message

	for _, m := range msgs {
		role := "user"
		if m.Role == vscodeprompt.RoleSystem {
			for _, part := range m.Content {
				systemPrompt += part.Text + "\n"
			}
			continue
		} else if m.Role == vscodeprompt.RoleAssistant {
			role = "assistant"
		}

		var contentBuilder strings.Builder
		for _, part := range m.Content {
			contentBuilder.WriteString(part.Text)
		}

		anthropicMsgs = append(anthropicMsgs, message{
			Role:    role,
			Content: contentBuilder.String(),
		})
	}

	return messageRequest{
		Model:       c.model,
		Messages:    anthropicMsgs,
		MaxTokens:   anthropicMaxTokens,
		System:      strings.TrimSpace(systemPrompt),
		Temperature: temperature,
		Stream:      stream,
	}
}

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	reqBody := c.buildRequest(msgs, temperature, false)

	headers := map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}

	var msgResp messageResponse
	if err := httpx.DoJSONRequest(ctx, c.client, "POST", c.baseURL, headers, reqBody, &msgResp); err != nil {
		return "", logger.LogError(err, "anthropic: request failed")
	}

	if len(msgResp.Content) == 0 {
		return "", logger.LogError(fmt.Errorf("empty response"), "anthropic: no content")
	}

	return msgResp.Content[0].Text, nil
}

// anthropicStreamChunk is a single SSE event from the streaming messages API.
// Only content_block_delta events carry text in delta.text.
type anthropicStreamChunk struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

// GenerateStream streams the completion using Anthropic's SSE protocol,
// invoking onDelta for each text chunk. It returns the full accumulated text.
func (c *Client) GenerateStream(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64, onDelta func(string)) (string, error) {
	reqBody := c.buildRequest(msgs, temperature, true)
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", logger.LogError(err, "anthropic: stream request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic: API error (status %d): %s", resp.StatusCode, string(body))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var chunk anthropicStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Type == "content_block_delta" && chunk.Delta.Text != "" {
			full.WriteString(chunk.Delta.Text)
			if onDelta != nil {
				onDelta(chunk.Delta.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", logger.LogError(err, "anthropic: stream read failed")
	}
	return full.String(), nil
}
