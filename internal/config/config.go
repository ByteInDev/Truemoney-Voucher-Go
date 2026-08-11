package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration for the API server.
type Config struct {
	Port int    // HTTP listen port (env PORT)
	Addr string // computed listen address, e.g. ":3000"
}

// Load reads configuration from the environment with sane defaults.
// It returns an error only when an explicitly-set value is invalid.
func Load() (*Config, error) {
	cfg := &Config{Port: 3000}

	if raw := os.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid PORT %q: must be a number between 1 and 65535", raw)
		}
		cfg.Port = port
	}
	cfg.Addr = fmt.Sprintf(":%d", cfg.Port)

	return cfg, nil
}