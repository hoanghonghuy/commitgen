package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/logger"
)

// maxRetries is the number of additional attempts made for transient failures.
const maxRetries = 2

// DoJSONRequest handles the common HTTP JSON request lifecycle:
// marshal body → create request → set headers → execute → check status → unmarshal response.
// Transient failures (network errors, HTTP 429 and 5xx) are retried with a
// simple linear backoff; client errors (4xx other than 429) are not retried.
func DoJSONRequest(ctx context.Context, client *http.Client, method string, url string, headers map[string]string, body interface{}, response interface{}) error {
	// Marshal request body once; reused across retry attempts.
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
			logger.Info("httpx: retry attempt", "attempt", attempt, "max", maxRetries, "url", url)
		}

		retry, err := doOnce(ctx, client, method, url, headers, b, response)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			return err
		}
	}
	return fmt.Errorf("httpx: failed after %d retries: %w", maxRetries, lastErr)
}

// doOnce performs a single request attempt. It returns (retryable, err).
func doOnce(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body []byte, response interface{}) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Network/transport errors are retryable, except when the context is done.
		if ctx.Err() != nil {
			return false, err
		}
		return true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		logger.Error("http: API error", "status", resp.StatusCode, "body", string(respBody))
		apiErr := fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		return isRetryableStatus(resp.StatusCode), apiErr
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}
	return false, nil
}

// isRetryableStatus reports whether an HTTP status code warrants a retry.
func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || (status >= 500 && status <= 599)
}
