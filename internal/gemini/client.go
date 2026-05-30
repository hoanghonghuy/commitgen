package gemini

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/httpx"
	"github.com/hoanghonghuy/commitgen/internal/logger"
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
		client: &http.Client{Timeout: 120 * time.Second},
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

func (c *Client) Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	return c.generate(ctx, msgs, temperature)
}

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	// Gemini: System instructions are separate. Roles are "user" and "model".

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
		GenerationConfig: generationConfig{
			Temperature: temperature,
		},
	}

	if len(systemParts) > 0 {
		reqBody.SystemInstruction = &content{
			Parts: systemParts,
		}
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.model, c.apiKey)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	var genResp generateContentResponse
	if err := httpx.DoJSONRequest(ctx, c.client, "POST", url, headers, reqBody, &genResp); err != nil {
		return "", logger.LogError(err, "gemini: request failed")
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", logger.LogError(fmt.Errorf("empty response"), "gemini: no content")
	}

	return genResp.Candidates[0].Content.Parts[0].Text, nil
}
