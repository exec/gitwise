package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	defaultAnthropicModel = "claude-sonnet-4-6"
	maxRetries          = 3
	baseRetryDelay      = 1 * time.Second
)

// AnthropicProvider implements Provider using the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewAnthropicProvider creates a provider that calls the Anthropic Messages API.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = defaultAnthropicModel
	}
	return &AnthropicProvider{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // LLM responses can be slow
		},
	}
}

func (p *AnthropicProvider) Name() string             { return "anthropic" }
func (p *AnthropicProvider) SupportsParallel() bool    { return true }

// anthropicRequest is the request body for the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the non-streaming response from the Anthropic Messages API.
type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// anthropicSSEEvent represents a single SSE event from the streaming API.
type anthropicSSEEvent struct {
	Type string
	Data json.RawMessage
}

// anthropicContentBlockDelta is the delta payload for content_block_delta events.
type anthropicContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// anthropicMessageDelta is the delta payload for message_delta events.
type anthropicMessageDelta struct {
	Type  string `json:"type"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// anthropicMessageStart is the payload for message_start events.
type anthropicMessageStart struct {
	Type    string `json:"type"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (p *AnthropicProvider) buildRequest(req GenerateRequest, stream bool) anthropicRequest {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	msgs := make([]anthropicMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = anthropicMessage{Role: m.Role, Content: m.Content}
	}

	return anthropicRequest{
		Model:     p.model,
		MaxTokens: maxTokens,
		System:    req.SystemPrompt,
		Messages:  msgs,
		Stream:    stream,
	}
}

// Generate sends a non-streaming request to the Anthropic Messages API.
func (p *AnthropicProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	apiReq := p.buildRequest(req, false)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	var resp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create anthropic request: %w", err)
		}
		p.setHeaders(httpReq)

		resp, err = p.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("anthropic request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			resp.Body.Close()
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt)))
			slog.Warn("anthropic rate limited, retrying", "attempt", attempt+1, "delay", delay)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
		break
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read anthropic response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("anthropic error (%s): %s", result.Error.Type, result.Error.Message)
	}

	var content string
	for _, block := range result.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &GenerateResponse{
		Content:      content,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}, nil
}

// GenerateStream sends a streaming request to the Anthropic Messages API.
// The returned channel emits StreamChunks as they arrive via SSE.
func (p *AnthropicProvider) GenerateStream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	apiReq := p.buildRequest(req, true)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	p.setHeaders(httpReq)

	// Use a client without timeout for streaming — context handles cancellation.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		var inputTokens int
		var outputTokens int

		scanner := bufio.NewScanner(resp.Body)
		// Allow up to 1MB per SSE line
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// SSE format: "event: <type>" followed by "data: <json>"
			if strings.HasPrefix(line, "event: ") {
				// Read the next data line
				eventType := strings.TrimPrefix(line, "event: ")
				if !scanner.Scan() {
					break
				}
				dataLine := scanner.Text()
				if !strings.HasPrefix(dataLine, "data: ") {
					continue
				}
				data := []byte(strings.TrimPrefix(dataLine, "data: "))

				switch eventType {
				case "message_start":
					var msg anthropicMessageStart
					if err := json.Unmarshal(data, &msg); err == nil {
						inputTokens = msg.Message.Usage.InputTokens
					}

				case "content_block_delta":
					var delta anthropicContentBlockDelta
					if err := json.Unmarshal(data, &delta); err == nil && delta.Delta.Type == "text_delta" {
						select {
						case ch <- StreamChunk{Content: delta.Delta.Text}:
						case <-ctx.Done():
							return
						}
					}

				case "message_delta":
					var delta anthropicMessageDelta
					if err := json.Unmarshal(data, &delta); err == nil {
						outputTokens = delta.Usage.OutputTokens
					}

				case "message_stop":
					select {
					case ch <- StreamChunk{
						Done:         true,
						InputTokens:  inputTokens,
						OutputTokens: outputTokens,
					}:
					case <-ctx.Done():
					}
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			slog.Warn("anthropic stream scanner error", "error", err)
		}
	}()

	return ch, nil
}

func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
}
