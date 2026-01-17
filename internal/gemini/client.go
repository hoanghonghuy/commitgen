package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

type Config struct {
	APIKey string
	Model  string
}

type Client struct {
	apiKey string
	model  string
	client *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		client: &http.Client{},
	}
}

// Minimal Gemini API structs
type generateContentRequest struct {
	Contents          []content        `json:"contents"`
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	GenerationConfig  generationConfig `json:"generationConfig,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"` // "user" or "model"
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generationConfig struct {
	Temperature float64 `json:"temperature,omitempty"`
}

type generateContentResponse struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content content `json:"content"`
}

func (c *Client) GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	// Gemini: System instructions are separate. Roles are "user" and "model".

	var systemParts []part
	var contents []content

	for _, m := range msgs {
		if m.Role == 3 { // System
			for _, p := range m.Content {
				systemParts = append(systemParts, part{Text: p.Text})
			}
			continue
		}

		role := "user"
		if m.Role == 2 { // Assistant
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
		GenerationConfig: generationConfig{
			Temperature: temperature,
		},
	}

	if len(systemParts) > 0 {
		reqBody.SystemInstruction = &content{
			Parts: systemParts,
		}
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(body))
	}

	var genResp generateContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from gemini")
	}

	return genResp.Candidates[0].Content.Parts[0].Text, nil
}
