package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestOllamaProvider_Embed(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %s", ct)
		}

		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if req.Model != "nomic-embed-text" {
			t.Errorf("unexpected model: %s", req.Model)
		}

		callCount.Add(1)
		embeddings := make([][]float32, len(req.Input))
		for i := range req.Input {
			embeddings[i] = []float32{0.1, 0.2, 0.3}
		}
		resp := ollamaEmbedResponse{Embeddings: embeddings}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(server.URL, "nomic-embed-text", 3)

	t.Run("single text", func(t *testing.T) {
		callCount.Store(0)
		embeddings, err := p.Embed(context.Background(), []string{"hello world"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(embeddings) != 1 {
			t.Fatalf("expected 1 embedding, got %d", len(embeddings))
		}
		if len(embeddings[0]) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(embeddings[0]))
		}
		if callCount.Load() != 1 {
			t.Errorf("expected 1 API call, got %d", callCount.Load())
		}
	})

	t.Run("batch texts", func(t *testing.T) {
		callCount.Store(0)
		embeddings, err := p.Embed(context.Background(), []string{"hello", "world", "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(embeddings) != 3 {
			t.Fatalf("expected 3 embeddings, got %d", len(embeddings))
		}
		if callCount.Load() != 1 {
			t.Errorf("expected 1 API call (batch), got %d", callCount.Load())
		}
	})

	t.Run("empty input", func(t *testing.T) {
		callCount.Store(0)
		embeddings, err := p.Embed(context.Background(), []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if embeddings != nil {
			t.Errorf("expected nil, got %v", embeddings)
		}
		if callCount.Load() != 0 {
			t.Errorf("expected 0 API calls, got %d", callCount.Load())
		}
	})
}

func TestOllamaProvider_EmbedError(t *testing.T) {
	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		defer server.Close()

		p := NewOllamaProvider(server.URL, "nomic-embed-text", 768)
		_, err := p.Embed(context.Background(), []string{"test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("ollama error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ollamaEmbedResponse{Error: "model not found"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p := NewOllamaProvider(server.URL, "nonexistent-model", 768)
		_, err := p.Embed(context.Background(), []string{"test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		p := NewOllamaProvider("http://localhost:1", "nomic-embed-text", 768)
		_, err := p.Embed(context.Background(), []string{"test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("count mismatch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ollamaEmbedResponse{Embeddings: [][]float32{{0.1}}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		p := NewOllamaProvider(server.URL, "nomic-embed-text", 768)
		_, err := p.Embed(context.Background(), []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error for count mismatch, got nil")
		}
	})
}

func TestOllamaProvider_Metadata(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434", "mxbai-embed-large", 1024)
	if p.Dimensions() != 1024 {
		t.Errorf("Dimensions() = %d, want 1024", p.Dimensions())
	}
	if p.ModelName() != "mxbai-embed-large" {
		t.Errorf("ModelName() = %q, want %q", p.ModelName(), "mxbai-embed-large")
	}
}
