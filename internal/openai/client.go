package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type chatRequest struct {
	Model       string                       `json:"model"`
	Messages    []vscodeprompt.OpenAIMessage `json:"messages"`
	Temperature float64                      `json:"temperature,omitempty"`
	Stream      bool                         `json:"stream,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error) {
	return c.generateWithRetry(ctx, msgs, temp, 2)
}

// endpoint returns the chat completions URL for the configured base URL.
func (c *Client) endpoint() string {
	return strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
}

// buildRequest assembles the chat completion request payload shared by the
// blocking and streaming code paths.
func (c *Client) buildRequest(msgs []vscodeprompt.VSCodeMessage, temp float64, stream bool) chatRequest {
	return chatRequest{
		Model:       c.cfg.Model,
		Messages:    vscodeprompt.ToOpenAIMessages(msgs),
		Temperature: temp,
		Stream:      stream,
	}
}

// GenerateStream streams the completion using SSE, invoking onDelta for each
// text chunk as it arrives. It returns the full accumulated message.
func (c *Client) GenerateStream(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64, onDelta func(string)) (string, error) {
	payload, err := json.Marshal(c.buildRequest(msgs, temp, true))
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.endpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", logger.LogError(err, "openai: stream request failed", "url", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai: API error (status %d): %s", resp.StatusCode, truncateString(string(body), 500))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk chatResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			delta = chunk.Choices[0].Message.Content
		}
		if delta != "" {
			full.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", logger.LogError(err, "openai: stream read failed")
	}

	result := full.String()
	if result == "" {
		return "", logger.LogError(fmt.Errorf("empty streaming response"), "openai: no content in stream")
	}
	return result, nil
}

// apiStatusError carries the HTTP status code from a non-2xx API response so
// retry logic can decide based on the status instead of matching error strings.
type apiStatusError struct {
	status int
	msg    string
}

func (e *apiStatusError) Error() string { return e.msg }

// isRetryableErr reports whether a failed request should be retried. API errors
// are retried only for 429 (rate limit) and 5xx (server) responses; client
// errors (e.g. 401/403/400/404) are not. Non-API errors (network/transport)
// are retried unless the context deadline was exceeded or the call timed out.
func isRetryableErr(err error) bool {
	var se *apiStatusError
	if errors.As(err, &se) {
		return se.status == http.StatusTooManyRequests || (se.status >= 500 && se.status <= 599)
	}
	msg := err.Error()
	if strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "Client.Timeout") {
		return false
	}
	return true
}

func (c *Client) generateWithRetry(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64, maxRetries int) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info("openai: retry attempt", "attempt", attempt, "max", maxRetries)
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		result, err := c.generate(ctx, msgs, temp)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isRetryableErr(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("openai: failed after %d retries: %v", maxRetries, lastErr)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error) {
	payload, err := json.Marshal(c.buildRequest(msgs, temp, false))
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.endpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", logger.LogError(err, "openai: request failed", "url", url)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	// Check HTTP status before parsing. Non-2xx responses (auth errors, rate
	// limits, gateway errors) may not be valid JSON, so handle them explicitly
	// and include the status code in the error (retry logic relies on it).
	if resp.StatusCode != http.StatusOK {
		var out chatResponse
		if jsonErr := json.Unmarshal(b, &out); jsonErr == nil && out.Error != nil {
			logger.Error("openai: API error", "status", resp.StatusCode, "message", out.Error.Message, "type", out.Error.Type)
			return "", &apiStatusError{
				status: resp.StatusCode,
				msg:    fmt.Sprintf("openai: API error (status %d): %s (%s)", resp.StatusCode, out.Error.Message, out.Error.Type),
			}
		}
		logger.Error("openai: API error", "status", resp.StatusCode, "body", truncateString(string(b), 500))
		return "", &apiStatusError{
			status: resp.StatusCode,
			msg:    fmt.Sprintf("openai: API error (status %d): %s", resp.StatusCode, truncateString(string(b), 500)),
		}
	}

	// Check if response is streaming (SSE format)
	responseStr := string(b)
	if strings.HasPrefix(responseStr, "data: ") {
		// Parse streaming response
		lines := strings.Split(responseStr, "\n")
		var content strings.Builder

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || line == "data: [DONE]" {
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				jsonStr := strings.TrimPrefix(line, "data: ")
				var chunk chatResponse
				if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
					continue
				}
				if len(chunk.Choices) > 0 {
					// Streaming uses delta, non-streaming uses message
					if chunk.Choices[0].Delta.Content != "" {
						content.WriteString(chunk.Choices[0].Delta.Content)
					} else if chunk.Choices[0].Message.Content != "" {
						content.WriteString(chunk.Choices[0].Message.Content)
					}
					// Skip reasoning_content — it's the model's internal thinking process,
					// not part of the final output we want to show.
				}
			}
		}

		result := content.String()
		if result == "" {
			// Log raw response for debugging empty responses
			logger.Error("openai: empty streaming response", "raw_response_prefix", truncateString(responseStr, 500))
			return "", logger.LogError(fmt.Errorf("empty streaming response"), "openai: no content in response")
		}
		return result, nil
	}

	// Non-streaming response
	var out chatResponse
	if err := json.Unmarshal(b, &out); err != nil {
		logger.Error("openai: decode error", "error", err, "response", string(b))
		return "", fmt.Errorf("decode error: %v\nraw: %s", err, string(b))
	}
	if out.Error != nil {
		logger.Error("openai: API error", "message", out.Error.Message, "type", out.Error.Type)
		return "", fmt.Errorf("llm error: %s (%s)", out.Error.Message, out.Error.Type)
	}
	if len(out.Choices) == 0 {
		return "", logger.LogError(fmt.Errorf("empty choices"), "openai: no choices in response")
	}
	return out.Choices[0].Message.Content, nil
}
