package embedding

import "context"

// NoopProvider returns nil embeddings. Used when no embedding API key is configured.
type NoopProvider struct{}

func (p *NoopProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	return result, nil
}

func (p *NoopProvider) Dimensions() int   { return 0 }
func (p *NoopProvider) ModelName() string  { return "noop" }
