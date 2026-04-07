package llm

import (
	"context"
	"fmt"
	"log/slog"
)

// Provider is the interface for LLM generative providers.
type Provider interface {
	// GenerateStream sends a request and returns a channel of streaming chunks.
	GenerateStream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error)
	// Generate sends a request and blocks until the full response is ready.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	// Name returns the provider identifier (e.g. "anthropic", "ollama_local").
	Name() string
	// SupportsParallel returns true if the provider can handle concurrent requests.
	SupportsParallel() bool
}

// GenerateRequest is the input to an LLM generation call.
type GenerateRequest struct {
	SystemPrompt string
	Messages     []Message
	MaxTokens    int
	Temperature  float64
}

// Message is a single turn in a conversation.
type Message struct {
	Role    string // "user", "assistant"
	Content string
}

// StreamChunk is a single piece of a streaming response.
type StreamChunk struct {
	Content      string
	Done         bool
	InputTokens  int // populated only on the final chunk
	OutputTokens int // populated only on the final chunk
}

// GenerateResponse is the complete result of a non-streaming generation.
type GenerateResponse struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// Gateway routes LLM requests to the configured provider.
type Gateway struct {
	provider Provider
	enabled  bool
}

// NewGateway creates a gateway with the given provider. Pass nil to disable.
func NewGateway(provider Provider) *Gateway {
	return &Gateway{
		provider: provider,
		enabled:  provider != nil,
	}
}

// IsEnabled returns true when a real provider is configured.
func (g *Gateway) IsEnabled() bool {
	return g.enabled
}

// Provider returns the underlying provider (may be nil if disabled).
func (g *Gateway) Provider() Provider {
	return g.provider
}

// Generate delegates to the configured provider. Returns an error if disabled.
func (g *Gateway) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if !g.enabled {
		return nil, fmt.Errorf("llm gateway is disabled (no provider configured)")
	}
	slog.Debug("llm generate", "provider", g.provider.Name(), "messages", len(req.Messages))
	return g.provider.Generate(ctx, req)
}

// GenerateStream delegates to the configured provider. Returns an error if disabled.
func (g *Gateway) GenerateStream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if !g.enabled {
		return nil, fmt.Errorf("llm gateway is disabled (no provider configured)")
	}
	slog.Debug("llm generate stream", "provider", g.provider.Name(), "messages", len(req.Messages))
	return g.provider.GenerateStream(ctx, req)
}
