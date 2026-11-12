package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
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
		cfg:  cfg,
		http: httpx.NewClient(cfg.Timeout),
	}
}

type chatRequest struct {
	Model       string                       `json:"model"`
	Messages    []vscodeprompt.OpenAIMessage `json:"messages"`
	Temperature float64                      `json:"temperature"`
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
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

var errIncompleteCompletion = errors.New("openai: incomplete completion")

func (r chatResponse) responseError() error {
	if r.Error != nil {
		return fmt.Errorf("openai: stream error: %s (%s)", r.Error.Message, r.Error.Type)
	}
	for _, choice := range r.Choices {
		if choice.FinishReason != "" && choice.FinishReason != "stop" {
			return fmt.Errorf("%w: finish_reason=%s", errIncompleteCompletion, choice.FinishReason)
		}
	}
	return nil
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close HTTP response", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai: API error (status %d): %s", resp.StatusCode, truncateString(string(body), 500))
	}
	return readStream(resp.Body, onDelta)
}

// readStream is shared by live streaming and compatible servers that return SSE
// for a blocking request. SSE permits one optional space after the field colon.
func readStream(body io.Reader, onDelta func(string)) (string, error) {
	var full strings.Builder
	var pending strings.Builder
	completed := false
	consumePending := func() error {
		if pending.Len() == 0 {
			return nil
		}
		var chunk chatResponse
		if err := json.Unmarshal([]byte(pending.String()), &chunk); err != nil {
			return fmt.Errorf("openai: invalid JSON in SSE event: %w", err)
		}
		pending.Reset()
		if err := chunk.responseError(); err != nil {
			return err
		}
		if len(chunk.Choices) == 0 {
			return nil
		}
		if chunk.Choices[0].FinishReason != "" {
			completed = true
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
		return nil
	}
	scanner := httpx.NewSSEScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := consumePending(); err != nil {
				return "", err
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
		if data == "[DONE]" {
			if pending.Len() > 0 && !json.Valid([]byte(pending.String())) {
				return "", fmt.Errorf("openai: incomplete SSE event: %w", io.ErrUnexpectedEOF)
			}
			if err := consumePending(); err != nil {
				return "", err
			}
			completed = true
			break
		}
		// Legacy compatible servers may omit blank separators between complete
		// JSON records. Wait for a new record before consuming the previous one,
		// so trailing whitespace data fields stay part of their original event.
		if pending.Len() > 0 && json.Valid([]byte(pending.String())) && json.Valid([]byte(data)) {
			if err := consumePending(); err != nil {
				return "", err
			}
		}
		if pending.Len() > 0 {
			pending.WriteByte('\n')
		}
		pending.WriteString(data)
	}
	if err := scanner.Err(); err != nil {
		return "", logger.LogError(err, "openai: stream read failed")
	}
	if pending.Len() > 0 {
		if !json.Valid([]byte(pending.String())) {
			return "", fmt.Errorf("openai: stream ended before completion: %w", io.ErrUnexpectedEOF)
		}
		if err := consumePending(); err != nil {
			return "", err
		}
	}
	if !completed {
		return "", fmt.Errorf("openai: stream ended before completion: %w", io.ErrUnexpectedEOF)
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
	if errors.Is(err, errIncompleteCompletion) {
		return false
	}
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
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
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
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close HTTP response", "error", err)
		}
	}()

	b, readErr := io.ReadAll(resp.Body)

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

	if readErr != nil {
		return "", fmt.Errorf("openai: read response: %w", readErr)
	}

	// Some compatible servers return SSE even for a blocking request.
	responseStr := string(b)
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") || strings.HasPrefix(responseStr, "data:") {
		return readStream(strings.NewReader(responseStr), nil)
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
	if err := out.responseError(); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", logger.LogError(fmt.Errorf("empty choices"), "openai: no choices in response")
	}
	return out.Choices[0].Message.Content, nil
}
