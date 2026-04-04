package embedding

import "context"

// Provider generates vector embeddings from text.
type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
	ModelName() string
}
