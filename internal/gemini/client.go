package gemini

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
	APIKey  string
	Model   string
	Timeout time.Duration
}

type Client struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

func New(cfg Config) *Client {
	return &Client{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: defaultGeminiBaseURL,
		client:  httpx.NewClient(cfg.Timeout),
	}
}

// Minimal Gemini API structs
type generateContentRequest struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"` // "user" or "model"
	Parts []part `json:"parts"`
}

type part struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"`
}

func (c content) text() string {
	var text strings.Builder
	for _, part := range c.Parts {
		if !part.Thought {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

type generationConfig struct {
	Temperature float64 `json:"temperature"`
}

type generateContentResponse struct {
	Candidates []candidate `json:"candidates"`
	Error      *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type candidate struct {
	Content      content `json:"content"`
	FinishReason string  `json:"finishReason"`
}

func (c candidate) completionError() error {
	if c.FinishReason != "" && c.FinishReason != "STOP" {
		return fmt.Errorf("gemini: incomplete completion: finishReason=%s", c.FinishReason)
	}
	return nil
}

func (c *Client) Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	return c.generate(ctx, msgs, temperature)
}

// buildRequest converts VSCode messages into Gemini's request format. System
// instructions are separate; roles map to "user" and "model".
func (c *Client) buildRequest(msgs []vscodeprompt.VSCodeMessage, temperature float64) generateContentRequest {
	var systemParts []part
	var contents []content

	for _, m := range msgs {
		if m.Role == vscodeprompt.RoleSystem {
			for _, p := range m.Content {
				systemParts = append(systemParts, part{Text: p.Text})
			}
			continue
		}

		role := "user"
		if m.Role == vscodeprompt.RoleAssistant {
			role = "model" // Gemini uses "model"
		}

		var parts []part
		for _, p := range m.Content {
			parts = append(parts, part{Text: p.Text})
		}

		contents = append(contents, content{
			Role:  role,
			Parts: parts,
		})
	}

	reqBody := generateContentRequest{
		Contents: contents,
		GenerationConfig: &generationConfig{
			Temperature: temperature,
		},
	}

	if len(systemParts) > 0 {
		reqBody.SystemInstruction = &content{
			Parts: systemParts,
		}
	}
	return reqBody
}

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	reqBody := c.buildRequest(msgs, temperature)

	// Pass the API key via header instead of the URL query string so it is
	// never written to logs (httpx logs the request URL on retry).
	url := fmt.Sprintf("%s/%s:generateContent", c.baseURL, c.model)
	headers := map[string]string{
		"Content-Type":   "application/json",
		"x-goog-api-key": c.apiKey,
	}

	var genResp generateContentResponse
	if err := httpx.DoJSONRequest(ctx, c.client, "POST", url, headers, reqBody, &genResp); err != nil {
		return "", logger.LogError(err, "gemini: request failed")
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", logger.LogError(fmt.Errorf("empty response"), "gemini: no content")
	}

	if err := genResp.Candidates[0].completionError(); err != nil {
		return "", err
	}
	return genResp.Candidates[0].Content.text(), nil
}

// GenerateStream streams the completion using Gemini's SSE endpoint
// (streamGenerateContent?alt=sse), invoking onDelta for each text chunk. It
// returns the full accumulated text.
func (c *Client) GenerateStream(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64, onDelta func(string)) (string, error) {
	reqBody := c.buildRequest(msgs, temperature)
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse", c.baseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", logger.LogError(err, "gemini: stream request failed")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close HTTP response", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(body))
	}

	var full strings.Builder
	completed := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var chunk generateContentResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return "", fmt.Errorf("gemini: stream error (%d): %s", chunk.Error.Code, chunk.Error.Message)
		}
		if len(chunk.Candidates) == 0 {
			continue
		}
		selected := chunk.Candidates[0]
		if err := selected.completionError(); err != nil {
			return "", err
		}
		text := selected.Content.text()
		if text != "" {
			full.WriteString(text)
			if onDelta != nil {
				onDelta(text)
			}
		}
		if selected.FinishReason == "STOP" {
			completed = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", logger.LogError(err, "gemini: stream read failed")
	}
	if !completed {
		return "", fmt.Errorf("gemini: stream ended before completion: %w", io.ErrUnexpectedEOF)
	}
	return full.String(), nil
}
