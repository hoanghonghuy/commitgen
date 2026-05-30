package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hoanghonghuy/commitgen/internal/logger"
)

// DoJSONRequest handles the common HTTP JSON request lifecycle:
// marshal body → create request → set headers → execute → check status → unmarshal response.
func DoJSONRequest(ctx context.Context, client *http.Client, method string, url string, headers map[string]string, body interface{}, response interface{}) error {
	// Marshal request body
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logger.Error("http: API error", "status", resp.StatusCode, "body", string(respBody))
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Unmarshal response
	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
