package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is loaded from ~/.config/dare/config.toml, then
// overridden by environment variables. This is what makes the gateway
// endpoint swappable (e.g. to your self-hosted n8n webhook) without
// recompiling the binary.
type Config struct {
	GatewayEndpoint string
	DefaultProvider string // "", "ollama", "gateway", "openrouter", "groq"
	Offline         bool
}

const defaultGatewayEndpoint = "https://gw.dare.dev/v1/chat"

func Load() Config {
	cfg := Config{GatewayEndpoint: defaultGatewayEndpoint}

	if path, err := configPath(); err == nil {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			parseSimpleTOML(f, &cfg)
		}
	}

	// Env vars always win — lets you point at self-hosted n8n for a single
	// session without touching the config file:
	//   DARE_GATEWAY_ENDPOINT=http://your-host:5678/webhook/dare dare run "..."
	if v := os.Getenv("DARE_GATEWAY_ENDPOINT"); v != "" {
		cfg.GatewayEndpoint = v
	}
	if v := os.Getenv("DARE_PROVIDER"); v != "" {
		cfg.DefaultProvider = v
	}
	if os.Getenv("DARE_OFFLINE") == "1" {
		cfg.Offline = true
	}

	return cfg
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dare", "config.toml"), nil
}

// parseSimpleTOML handles flat `key = "value"` / `key = true` lines only —
// deliberately minimal so the CLI has zero third-party dependencies and
// builds offline. Swap for a real TOML library if the config grows nested
// tables.
func parseSimpleTOML(f *os.File, cfg *Config) {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "gateway_endpoint":
			cfg.GatewayEndpoint = val
		case "default_provider":
			cfg.DefaultProvider = val
		case "offline":
			if b, err := strconv.ParseBool(val); err == nil {
				cfg.Offline = b
			}
		}
	}
}
