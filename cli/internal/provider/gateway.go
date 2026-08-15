package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GatewayProvider is the keyless fallback: a server-side proxy that holds
// real provider credentials so the client binary never needs one. This is
// what guarantees "no forced API key after install."
type GatewayProvider struct {
	Endpoint string // e.g. https://gw.dare.dev/v1/chat
	http     *http.Client
}

func NewGatewayProvider(endpoint string) *GatewayProvider {
	return &GatewayProvider{
		Endpoint: endpoint,
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

func (g *GatewayProvider) ID() string { return "gateway" }

func (g *GatewayProvider) Available(ctx context.Context) bool {
	// Gateway is treated as always-available last resort; a real
	// implementation would hit /health with a short timeout.
	return g.Endpoint != ""
}

func (g *GatewayProvider) Complete(ctx context.Context, req Request) (Response, error) {
	start := time.Now()
	body, _ := json.Marshal(map[string]any{
		"system": req.SystemPrompt,
		"prompt": req.UserPrompt,
		// no api_key field — the gateway injects its own server-side.
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Client", "dare-cli")

	resp, err := g.http.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("gateway returned %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Text  string `json:"text"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, err
	}

	return Response{
		Text:       out.Text,
		Model:      out.Model,
		ProviderID: g.ID(),
		LatencyMS:  time.Since(start).Milliseconds(),
	}, nil
}
