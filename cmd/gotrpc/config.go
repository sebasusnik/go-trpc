package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = "gotrpc.json"

// GoTRPCConfig represents the gotrpc.json configuration file.
type GoTRPCConfig struct {
	Source string `json:"source"` // Go source directory (e.g., "./api")
	Output string `json:"output"` // Output .d.ts path (e.g., "./web/src/generated/router.d.ts")
	Router string `json:"router"` // Router type name (default: "AppRouter")
}

// LoadConfig reads gotrpc.json from the given directory.
func LoadConfig(dir string) (*GoTRPCConfig, error) {
	path := filepath.Join(dir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", configFileName, err)
	}

	var cfg GoTRPCConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", configFileName, err)
	}

	if cfg.Source == "" {
		return nil, fmt.Errorf("%s: \"source\" is required", configFileName)
	}
	if cfg.Output == "" {
		return nil, fmt.Errorf("%s: \"output\" is required", configFileName)
	}
	if cfg.Router == "" {
		cfg.Router = "AppRouter"
	}

	// Resolve relative paths against the config directory
	if !filepath.IsAbs(cfg.Source) {
		cfg.Source = filepath.Join(dir, cfg.Source)
	}
	if !filepath.IsAbs(cfg.Output) {
		cfg.Output = filepath.Join(dir, cfg.Output)
	}

	return &cfg, nil
}
