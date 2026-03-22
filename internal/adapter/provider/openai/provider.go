package openai

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

	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// Config holds OpenAI provider configuration
type Config struct {
	Endpoint   string
	Model      string
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
}

// Provider implements port.LLMProvider for OpenAI-compatible endpoints
type Provider struct {
	config     Config
	httpClient *http.Client
}

// NewProvider creates a new OpenAI provider
// Panics if required configuration is missing
func NewProvider(cfg Config) *Provider {
	if cfg.Endpoint == "" {
		panic("openai provider: endpoint is required")
	}
	if cfg.Model == "" {
		panic("openai provider: model is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	return &Provider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// StreamCompletion streams tokens from the OpenAI-compatible endpoint
func (p *Provider) StreamCompletion(ctx context.Context, agentID string, prompt string) <-chan port.StreamToken {
	ch := make(chan port.StreamToken)

	go func() {
		defer close(ch)

		// Build request body
		reqBody := map[string]interface{}{
			"model":    p.config.Model,
			"messages": []map[string]string{{"role": "user", "content": prompt}},
			"stream":   true,
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			ch <- port.StreamToken{Error: fmt.Errorf("failed to marshal request: %w", err)}
			return
		}

		// Execute request with retry logic
		// Note: Must recreate http.Request for each retry because request body can only be read once
		var resp *http.Response
		for attempt := 0; attempt < p.config.MaxRetries; attempt++ {
			// Create request (must be recreated for each retry)
			req, err := http.NewRequestWithContext(ctx, "POST", p.config.Endpoint, bytes.NewReader(jsonBody))
			if err != nil {
				ch <- port.StreamToken{Error: fmt.Errorf("failed to create request: %w", err)}
				return
			}

			req.Header.Set("Content-Type", "application/json")
			if p.config.APIKey != "" {
				req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
			}

			resp, err = p.httpClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				break
			}

			if err != nil {
				// Check if error is retryable
				if !isRetryableError(err) || attempt == p.config.MaxRetries-1 {
					ch <- port.StreamToken{Error: fmt.Errorf("request failed after %d attempts: %w", attempt+1, err)}
					return
				}
			} else if resp.StatusCode == http.StatusTooManyRequests {
				// 429 - apply backoff
				resp.Body.Close()
				if attempt < p.config.MaxRetries-1 {
					backoff := time.Duration(attempt+1) * time.Second
					select {
					case <-ctx.Done():
						ch <- port.StreamToken{Error: ctx.Err()}
						return
					case <-time.After(backoff):
						continue
					}
				}
			} else {
				// Non-retryable HTTP error
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				ch <- port.StreamToken{Error: fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))}
				return
			}

			// Wait before retry
			if attempt < p.config.MaxRetries-1 {
				select {
				case <-ctx.Done():
					ch <- port.StreamToken{Error: ctx.Err()}
					return
				case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
				}
			}
		}

		if resp == nil {
			ch <- port.StreamToken{Error: fmt.Errorf("no response received")}
			return
		}

		defer resp.Body.Close()

		// Parse SSE stream
		scanner := bufio.NewScanner(resp.Body)
		emittedToken := false

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				ch <- port.StreamToken{Error: ctx.Err()}
				return
			default:
			}

			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Parse SSE data line
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Check for stream end
			if data == "[DONE]" {
				break
			}

			// Parse JSON
			var streamResp struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				// Log parsing error but continue with partial result
				continue
			}

			if len(streamResp.Choices) > 0 {
				content := streamResp.Choices[0].Delta.Content
				if content != "" {
					emittedToken = true
					ch <- port.StreamToken{Token: content, Done: false}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			// Stream error - return partial result if we have content
			if emittedToken {
				ch <- port.StreamToken{Done: true}
				return
			}
			ch <- port.StreamToken{Error: fmt.Errorf("stream error: %w", err)}
			return
		}

		// Send completion signal
		ch <- port.StreamToken{Done: true}
	}()

	return ch
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	// Check for common network errors
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "temporary") ||
		strings.Contains(errStr, "eof")
}
