package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	APIBaseURL string `json:"api_base_url"`
	Token      string `json:"token"`
}

func newAppConfig() *AppConfig {
	return &AppConfig{APIBaseURL: "http://localhost:8080"}
}

func loadConfig(path string) (*AppConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "http://localhost:8080"
	}
	return &cfg, nil
}

func saveConfig(path string, cfg *AppConfig) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func (c *AppConfig) clone() *AppConfig {
	if c == nil {
		return newAppConfig()
	}
	cp := *c
	if cp.APIBaseURL == "" {
		cp.APIBaseURL = "http://localhost:8080"
	}
	return &cp
}
