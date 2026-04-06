package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaProvider generates embeddings via a local Ollama instance.
type OllamaProvider struct {
	baseURL    string
	model      string
	dimensions int
	httpClient *http.Client
}

// NewOllamaProvider creates a provider that connects to an Ollama API.
// baseURL is the Ollama server URL (e.g. "http://localhost:11434").
// model is the embedding model name (e.g. "nomic-embed-text").
// dimensions is the expected vector dimension for the chosen model.
func NewOllamaProvider(baseURL, model string, dimensions int) *OllamaProvider {
	return &OllamaProvider{
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
		httpClient: &http.Client{},
	}
}

func (p *OllamaProvider) Dimensions() int  { return p.dimensions }
func (p *OllamaProvider) ModelName() string { return p.model }

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

func (p *OllamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Ollama's /api/embeddings endpoint handles one text at a time,
	// so we loop over the inputs.
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := p.embedSingle(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed text [%d]: %w", i, err)
		}
		embeddings[i] = vec
	}

	return embeddings, nil
}

func (p *OllamaProvider) embedSingle(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(ollamaRequest{
		Model:  p.model,
		Prompt: text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := p.baseURL + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("ollama error: %s", result.Error)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}

	return result.Embedding, nil
}
