package router

import (
	"context"
	"fmt"
	"time"

	"github.com/baalebos-cloud/dare/cli/internal/config"
	"github.com/baalebos-cloud/dare/cli/internal/provider"
)

type Router struct {
	Chain []provider.Provider // ordered: first available wins, next on error
	Debug bool
}

// New builds the provider chain from config: local providers first, then
// the gateway (which may be a self-hosted n8n webhook, a deployed proxy,
// or anything speaking the same request/response shape) — unless the user
// has forced --offline, in which case the gateway is never added at all.
func New(cfg config.Config, debug bool) *Router {
	chain := []provider.Provider{
		provider.NewOllamaProvider(),
		// llama.cpp / LocalAI providers would be appended here, same pattern.
	}
	if !cfg.Offline {
		chain = append(chain, provider.NewGatewayProvider(cfg.GatewayEndpoint))
	}
	return &Router{Chain: chain, Debug: debug}
}

// Resolve picks the first available provider without making a completion
// call yet (cheap discovery, cached by the caller for the session).
func (r *Router) Resolve(ctx context.Context) (provider.Provider, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	for _, p := range r.Chain {
		if p.Available(probeCtx) {
			if r.Debug {
				fmt.Printf("[router] selected provider: %s\n", p.ID())
			}
			return p, nil
		}
	}
	return nil, fmt.Errorf("no provider available (local and gateway both unreachable)")
}

// Complete tries providers in order, falling over to the next on failure —
// this is the actual runtime fallback behavior, distinct from Resolve's
// cheap discovery pass.
func (r *Router) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	var lastErr error
	tried := false
	for _, p := range r.Chain {
		if !p.Available(ctx) {
			continue
		}
		tried = true
		resp, err := p.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if r.Debug {
			fmt.Printf("[router] provider %s failed: %v — falling back\n", p.ID(), err)
		}
	}
	if !tried {
		return provider.Response{}, fmt.Errorf("no provider was reachable (local inference not detected, gateway unreachable or disabled)")
	}
	return provider.Response{}, fmt.Errorf("all providers failed: %w", lastErr)
}
