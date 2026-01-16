package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type chatReq struct {
	Model       string                       `json:"model"`
	Messages    []vscodeprompt.OpenAIMessage `json:"messages"`
	Temperature float64                      `json:"temperature,omitempty"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Ensure Client implements ai.Provider
// var _ ai.Provider = (*Client)(nil) // Cyclic dependency if we import ai here?
// No, ai imports vscodeprompt. openai imports vscodeprompt.
// We need openai to import ai to verify implementation?
// Actually, run.go will rely on the interface defined in ai. openai just needs to have the method.
// To avoid circular imports if ai/provider.go imports vscodeprompt (which is fine),
// openai package can rely on vscodeprompt too.
// But checking "implements" requires importing "github.com/hoanghonghuy/commitgen/internal/ai".
// That should be fine: internal/ai -> vscodeprompt. internal/openai -> vscodeprompt. internal/openai -> internal/ai.
// The dependency graph is DAG.

func (c *Client) GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error) {
	oaiMsgs := vscodeprompt.ToOpenAIMessages(msgs)

	base := strings.TrimRight(c.cfg.BaseURL, "/")
	url := base + "/chat/completions"

	payload, _ := json.Marshal(chatReq{
		Model:       c.cfg.Model,
		Messages:    oaiMsgs,
		Temperature: temp,
	})

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	var out chatResp
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("decode error: %v\nraw: %s", err, string(b))
	}
	if out.Error != nil {
		return "", fmt.Errorf("llm error: %s (%s)", out.Error.Message, out.Error.Type)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm: empty choices")
	}
	return out.Choices[0].Message.Content, nil
}
