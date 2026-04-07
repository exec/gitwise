package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const defaultOllamaModel = "llama3"

// ollamaChatRequest is the request body for Ollama's /api/chat endpoint.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Options  *ollamaChatOptions  `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// ollamaChatResponse is a single response object from Ollama's /api/chat endpoint.
// In streaming mode, multiple JSON objects are returned (one per line).
type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done               bool   `json:"done"`
	Error              string `json:"error,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
}

// OllamaLocalProvider connects to a local Ollama instance.
// Only one request runs at a time (SupportsParallel = false).
type OllamaLocalProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaLocalProvider creates a provider for a local Ollama instance.
func NewOllamaLocalProvider(baseURL, model string) *OllamaLocalProvider {
	if model == "" {
		model = defaultOllamaModel
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaLocalProvider{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (p *OllamaLocalProvider) Name() string          { return "ollama_local" }
func (p *OllamaLocalProvider) SupportsParallel() bool { return false }

func (p *OllamaLocalProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return ollamaGenerate(ctx, p.httpClient, p.baseURL, p.model, req)
}

func (p *OllamaLocalProvider) GenerateStream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	return ollamaGenerateStream(ctx, p.baseURL, p.model, req)
}

// OllamaCloudProvider connects to a remote Ollama cloud endpoint.
// Supports parallel requests.
type OllamaCloudProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaCloudProvider creates a provider for a remote Ollama cloud endpoint.
func NewOllamaCloudProvider(baseURL, model string) *OllamaCloudProvider {
	if model == "" {
		model = defaultOllamaModel
	}
	return &OllamaCloudProvider{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (p *OllamaCloudProvider) Name() string          { return "ollama_cloud" }
func (p *OllamaCloudProvider) SupportsParallel() bool { return true }

func (p *OllamaCloudProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	return ollamaGenerate(ctx, p.httpClient, p.baseURL, p.model, req)
}

func (p *OllamaCloudProvider) GenerateStream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	return ollamaGenerateStream(ctx, p.baseURL, p.model, req)
}

// ollamaBuildMessages converts a GenerateRequest into Ollama chat messages.
// The system prompt becomes a "system" role message prepended to the conversation.
func ollamaBuildMessages(req GenerateRequest) []ollamaChatMessage {
	msgs := make([]ollamaChatMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, ollamaChatMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, ollamaChatMessage{Role: m.Role, Content: m.Content})
	}
	return msgs
}

func ollamaBuildOptions(req GenerateRequest) *ollamaChatOptions {
	opts := &ollamaChatOptions{}
	if req.Temperature > 0 {
		opts.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		opts.NumPredict = req.MaxTokens
	}
	if opts.Temperature == 0 && opts.NumPredict == 0 {
		return nil
	}
	return opts
}

// ollamaGenerate is the shared non-streaming implementation.
func ollamaGenerate(ctx context.Context, client *http.Client, baseURL, model string, req GenerateRequest) (*GenerateResponse, error) {
	apiReq := ollamaChatRequest{
		Model:    model,
		Messages: ollamaBuildMessages(req),
		Stream:   false,
		Options:  ollamaBuildOptions(req),
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	url := baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal ollama response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	return &GenerateResponse{
		Content:      result.Message.Content,
		InputTokens:  result.PromptEvalCount,
		OutputTokens: result.EvalCount,
	}, nil
}

// ollamaGenerateStream is the shared streaming implementation.
// Ollama streams JSON objects, one per line (not SSE).
func ollamaGenerateStream(ctx context.Context, baseURL, model string, req GenerateRequest) (<-chan StreamChunk, error) {
	apiReq := ollamaChatRequest{
		Model:    model,
		Messages: ollamaBuildMessages(req),
		Stream:   true,
		Options:  ollamaBuildOptions(req),
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	url := baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// No timeout for streaming — context handles cancellation.
	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk ollamaChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				slog.Warn("ollama stream: unmarshal error", "error", err)
				continue
			}

			if chunk.Error != "" {
				slog.Warn("ollama stream error", "error", chunk.Error)
				return
			}

			if chunk.Done {
				select {
				case ch <- StreamChunk{
					Done:         true,
					InputTokens:  chunk.PromptEvalCount,
					OutputTokens: chunk.EvalCount,
				}:
				case <-ctx.Done():
				}
				return
			}

			if chunk.Message.Content != "" {
				select {
				case ch <- StreamChunk{Content: chunk.Message.Content}:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			slog.Warn("ollama stream scanner error", "error", err)
		}
	}()

	return ch, nil
}
