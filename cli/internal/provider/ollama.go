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

const defaultOllamaHost = "http://127.0.0.1:11434"

type OllamaProvider struct {
	Host  string
	Model string // e.g. "qwen2.5-coder:7b" — auto-picked if empty
	http  *http.Client
}

func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{
		Host: defaultOllamaHost,
		http: &http.Client{Timeout: 400 * time.Millisecond},
	}
}

func (o *OllamaProvider) ID() string { return "ollama" }

// Available pings /api/tags with a short timeout — this is what makes
// "Local Execution Mode (Default)" auto-detection cheap and non-blocking.
func (o *OllamaProvider) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.Host+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	// Pick a code-capable model if none configured, preferring coder variants.
	if o.Model == "" {
		var tags struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if json.NewDecoder(resp.Body).Decode(&tags) == nil {
			for _, m := range tags.Models {
				o.Model = m.Name // first available; refine with a "coder"/"qwen" preference pass if desired
				break
			}
		}
	}
	return o.Model != ""
}

func (o *OllamaProvider) Complete(ctx context.Context, req Request) (Response, error) {
	start := time.Now()
	body, _ := json.Marshal(map[string]any{
		"model":  o.Model,
		"prompt": req.SystemPrompt + "\n\n" + req.UserPrompt,
		"stream": false,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, err
	}

	return Response{
		Text:       out.Response,
		Model:      o.Model,
		ProviderID: o.ID(),
		LatencyMS:  time.Since(start).Milliseconds(),
	}, nil
}
