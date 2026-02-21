package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	AddyIO  AddyIOConfig  `yaml:"addyio"`
	KeePass KeePassConfig `yaml:"keepass"`
	IMAP    IMAPConfig    `yaml:"imap"`
	DB      DBConfig      `yaml:"db"`
	Notify  NotifyConfig  `yaml:"notify"`
}

type AddyIOConfig struct {
	APIKey      string `yaml:"api_key"`
	BaseURL     string `yaml:"base_url"`
	AliasDomain string `yaml:"alias_domain"`
}

type KeePassConfig struct {
	DBPath      string `yaml:"db_path"`
	PasswordEnv string `yaml:"password_env"`
}

type IMAPConfig struct {
	Server      string `yaml:"server"`
	Port        int    `yaml:"port"`
	Username    string `yaml:"username"`
	PasswordEnv string `yaml:"password_env"`
	Folder      string `yaml:"folder"`
	TLS         bool   `yaml:"tls"`
}

// Password returns the IMAP password from the environment variable.
func (c IMAPConfig) Password() string {
	if c.PasswordEnv == "" {
		return ""
	}
	return os.Getenv(c.PasswordEnv)
}

type DBConfig struct {
	Path string `yaml:"path"`
}

type NotifyConfig struct {
	NtfyURL   string `yaml:"ntfy_url"`
	NtfyToken string `yaml:"ntfy_token"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Environment variable overrides
	if v := os.Getenv("ADDYIO_API_KEY"); v != "" {
		cfg.AddyIO.APIKey = v
	}

	// Expand ~ in paths
	cfg.DB.Path = expandHome(cfg.DB.Path)
	cfg.KeePass.DBPath = expandHome(cfg.KeePass.DBPath)

	// Defaults
	if cfg.AddyIO.BaseURL == "" {
		cfg.AddyIO.BaseURL = "https://app.addy.io"
	}
	if cfg.IMAP.Port == 0 {
		cfg.IMAP.Port = 993
	}
	if cfg.IMAP.Folder == "" {
		cfg.IMAP.Folder = "INBOX"
	}

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
