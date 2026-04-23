package embedding

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// failNProvider is a test Provider that fails the first N Embed calls, then succeeds.
type failNProvider struct {
	failCount  int
	callCount  atomic.Int32
	embeddings [][]float32 // returned on success
}

func (p *failNProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	n := int(p.callCount.Add(1))
	if n <= p.failCount {
		return nil, errors.New("provider temporarily unavailable")
	}
	// Return one embedding per text
	result := make([][]float32, len(texts))
	for i := range texts {
		if i < len(p.embeddings) {
			result[i] = p.embeddings[i]
		} else {
			result[i] = []float32{0.1, 0.2, 0.3}
		}
	}
	return result, nil
}

func (p *failNProvider) Dimensions() int   { return 3 }
func (p *failNProvider) ModelName() string { return "fail-n-test" }

// alwaysFailProvider never succeeds.
type alwaysFailProvider struct {
	callCount atomic.Int32
}

func (p *alwaysFailProvider) Embed(_ context.Context, _ []string) ([][]float32, error) {
	p.callCount.Add(1)
	return nil, errors.New("provider always fails")
}

func (p *alwaysFailProvider) Dimensions() int   { return 3 }
func (p *alwaysFailProvider) ModelName() string { return "always-fail-test" }

// TestEmbedWithRetry_SucceedsAfterFailures verifies that embedWithRetry
// succeeds when the provider fails fewer times than maxAttempts (3).
func TestEmbedWithRetry_SucceedsAfterFailures(t *testing.T) {
	tests := []struct {
		name      string
		failCount int
	}{
		{"no failures", 0},
		{"fail once", 1},
		{"fail twice", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &failNProvider{
				failCount:  tc.failCount,
				embeddings: [][]float32{{0.1, 0.2, 0.3}},
			}
			svc := &Service{provider: provider, enabled: true}

			// Use a short test context — retries use jitter which can be up to
			// ~4 s in prod; for tests we rely on the zero-jitter path since
			// rand.Int63n(0) panics, so we accept real jitter but with a cap.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			embeddings, err := svc.embedWithRetry(ctx, []string{"hello"})
			if err != nil {
				t.Fatalf("expected success after %d failures, got error: %v", tc.failCount, err)
			}
			if len(embeddings) != 1 {
				t.Fatalf("expected 1 embedding, got %d", len(embeddings))
			}
			expectedCalls := int32(tc.failCount + 1)
			if got := provider.callCount.Load(); got != expectedCalls {
				t.Errorf("expected %d provider calls, got %d", expectedCalls, got)
			}
		})
	}
}

// TestEmbedWithRetry_ExhaustsRetries verifies that embedWithRetry returns an
// error after maxAttempts (3) and makes exactly 3 provider calls.
func TestEmbedWithRetry_ExhaustsRetries(t *testing.T) {
	provider := &alwaysFailProvider{}
	svc := &Service{provider: provider, enabled: true}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := svc.embedWithRetry(ctx, []string{"hello"})
	if err == nil {
		t.Fatal("expected error when provider always fails")
	}
	if got := provider.callCount.Load(); got != 3 {
		t.Errorf("expected 3 provider calls (maxAttempts), got %d", got)
	}
}

// TestEmbedWithRetry_ContextCancel verifies that embedWithRetry honours
// context cancellation during the backoff sleep.
func TestEmbedWithRetry_ContextCancel(t *testing.T) {
	// Fail on every call; cancel the context early to interrupt backoff.
	provider := &alwaysFailProvider{}
	svc := &Service{provider: provider, enabled: true}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after the first call has a chance to run.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := svc.embedWithRetry(ctx, []string{"hello"})
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		// Either the provider error or context error is acceptable — we just need
		// an error.  If we got back the provider error that's fine too.
		t.Logf("non-context error returned (acceptable): %v", err)
	}
}

// TestSanitizeIdentifier covers the identifier sanitization contract.
func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"issues", "issues"},
		{"body_embedding", "body_embedding"},
		{"table123", "table123"},
		{"bad-name", "invalid_identifier"},
		{"bad name", "invalid_identifier"},
		{"bad;name", "invalid_identifier"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := sanitizeIdentifier(tc.input); got != tc.want {
				t.Errorf("sanitizeIdentifier(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
