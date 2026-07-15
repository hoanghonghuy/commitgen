package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/ai"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// generateCommitMessage runs a single generation pass: it optionally appends the
// Conventional Commits reminder, calls the provider, and extracts the message
// from a fenced code block (falling back to the raw text). Shared by the TUI and
// the non-interactive path.
func generateCommitMessage(ctx context.Context, provider ai.Provider, msgs []vscodeprompt.VSCodeMessage, temp float64, conventional bool) (string, error) {
	currentMsgs := make([]vscodeprompt.VSCodeMessage, len(msgs))
	copy(currentMsgs, msgs)

	if conventional {
		currentMsgs = append(currentMsgs, conventionalReminder())
	}

	raw, err := provider.Generate(ctx, currentMsgs, clampTemperature(temp))
	if err != nil {
		return "", err
	}
	msg, ok := vscodeprompt.ExtractOneTextCodeBlock(raw)
	if !ok {
		msg = raw
	}
	return msg, nil
}

// conventionalReminder returns the user message that instructs the model to
// follow the Conventional Commits specification.
func conventionalReminder() vscodeprompt.VSCodeMessage {
	return vscodeprompt.VSCodeMessage{
		Role: vscodeprompt.RoleUser,
		Content: []vscodeprompt.VSCodeContentPart{
			{Type: 1, Text: "CRITICAL INSTRUCTION: You must strictly follow the Conventional Commits specification (e.g. 'feat: add spinner', 'fix: resolve bug').\nDo not just describe the change; prefix it with the type."},
		},
	}
}

// runSuggestNonInteractive generates a commit message once and prints it to
// stdout without launching the TUI. With --print it also writes the hook file
// when one is configured; with --dry-run it never produces side effects.
func runSuggestNonInteractive(ctx context.Context, cfg Config, repoRoot string, provider ai.Provider, msgs []vscodeprompt.VSCodeMessage, tr *i18n.Translator) error {
	cctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	count := cfg.Count
	if count < 1 {
		count = 1
	}

	messages := make([]string, 0, count)
	for i := 0; i < count; i++ {
		msg, err := generateCommitMessage(cctx, provider, msgs, cfg.Temperature, cfg.Conventional)
		if err != nil {
			return logger.LogError(err, "failed to generate commit message")
		}
		messages = append(messages, strings.TrimSpace(msg))
	}

	if count == 1 {
		fmt.Println(messages[0])
	} else {
		for i, m := range messages {
			fmt.Printf("%d. %s\n", i+1, m)
		}
	}

	if cfg.DryRun {
		return nil
	}
	// --print (without --dry-run): write the first candidate to the hook file
	// when running inside a git hook.
	if cfg.HookFile != "" {
		if err := os.WriteFile(cfg.HookFile, []byte(messages[0]), 0644); err != nil {
			return logger.LogError(err, "failed to write hook file", "path", cfg.HookFile)
		}
	}
	return nil
}

// runPing verifies provider connectivity and credentials with a minimal request.
func runPing(ctx context.Context, cfg Config, tr *i18n.Translator) error {
	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}

	timeout := cfg.Timeout
	if timeout == 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	msgs := []vscodeprompt.VSCodeMessage{
		{Role: vscodeprompt.RoleUser, Content: []vscodeprompt.VSCodeContentPart{{Type: 1, Text: "ping"}}},
	}
	if _, err := provider.Generate(cctx, msgs, 0); err != nil {
		return logger.LogError(err, "ping failed", "provider", providerLabel(cfg.Provider))
	}
	fmt.Println(tr.T("ping.ok", providerLabel(cfg.Provider), cfg.Model))
	return nil
}

// runModels lists available models for providers that support discovery
// (Ollama and OpenAI-compatible endpoints).
func runModels(ctx context.Context, cfg Config, tr *i18n.Translator) error {
	switch strings.ToLower(cfg.Provider) {
	case "ollama":
		return listOllamaModels(ctx, cfg, tr)
	case "openai", "":
		return listOpenAIModels(ctx, cfg, tr)
	default:
		return fmt.Errorf("%s", tr.T("models.unsupported", providerLabel(cfg.Provider)))
	}
}

func providerLabel(p string) string {
	if strings.TrimSpace(p) == "" {
		return "openai"
	}
	return strings.ToLower(p)
}

func listOllamaModels(ctx context.Context, cfg Config, tr *i18n.Translator) error {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	var resp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := getJSON(ctx, base+"/api/tags", nil, &resp); err != nil {
		return logger.LogError(err, "failed to list ollama models")
	}
	names := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		names = append(names, m.Name)
	}
	printModels(names, tr)
	return nil
}

func listOpenAIModels(ctx context.Context, cfg Config, tr *i18n.Translator) error {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	headers := map[string]string{}
	if strings.TrimSpace(cfg.APIKey) != "" {
		headers["Authorization"] = "Bearer " + cfg.APIKey
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, base+"/models", headers, &resp); err != nil {
		return logger.LogError(err, "failed to list openai models")
	}
	names := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		names = append(names, m.ID)
	}
	printModels(names, tr)
	return nil
}

func printModels(names []string, tr *i18n.Translator) {
	if len(names) == 0 {
		fmt.Println(tr.T("models.none"))
		return
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}
}

// getJSON performs a GET request and decodes a JSON response.
func getJSON(ctx context.Context, url string, headers map[string]string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
