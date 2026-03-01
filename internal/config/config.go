package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	IMAP   IMAPConfig   `yaml:"imap"`
	DB     DBConfig     `yaml:"db"`
	Notify NotifyConfig `yaml:"notify"`
}

type IMAPConfig struct {
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Folder   string `yaml:"folder"`
	TLS      bool   `yaml:"tls"`
	password string `yaml:"-"`
}

func (c IMAPConfig) Password() string {
	return c.password
}

type DBConfig struct {
	Path string `yaml:"path"`
}

type NotifyConfig struct {
	NtfyURL       string `yaml:"ntfy_url"`
	NtfyToken     string `yaml:"ntfy_token"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := Config{
		IMAP: IMAPConfig{
			Port: 993,
			Folder: "INBOX",
			TLS: true,
		},
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Expand ~ in paths
	cfg.DB.Path = expandHome(cfg.DB.Path)
	cfg.IMAP.Server = envOrFallback("IMAP_SERVER", cfg.IMAP.Server)
	cfg.IMAP.Username = envOrFallback("IMAP_USERNAME", cfg.IMAP.Username)
	cfg.IMAP.password = envOrFallback("IMAP_PASSWORD", "")
	cfg.Notify.NtfyURL = envOrFallback("NTFY_URL", cfg.Notify.NtfyURL)
	cfg.Notify.NtfyToken = envOrFallback("NTFY_TOKEN", cfg.Notify.NtfyToken)

	return &cfg, nil
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

func envOrFallback(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}
