package provider

import "context"

// Request is the normalized payload every provider receives.
type Request struct {
	SystemPrompt string
	UserPrompt   string // rendered from captured log/traceback/context
}

// Response is the normalized completion returned by any provider.
type Response struct {
	Text       string
	Model      string
	ProviderID string
	LatencyMS  int64
}

// Provider is the pluggable backend contract. Add a new backend by
// implementing this interface and registering it in router.go.
type Provider interface {
	// ID is a short stable name, e.g. "ollama", "gateway", "groq".
	ID() string
	// Available performs a cheap health check (used during discovery).
	Available(ctx context.Context) bool
	// Complete sends the request and returns a normalized response.
	Complete(ctx context.Context, req Request) (Response, error)
}
