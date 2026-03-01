package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_IMAPFromUppercaseEnv(t *testing.T) {
	t.Setenv("IMAP_SERVER", "imap.env.example")
	t.Setenv("IMAP_USERNAME", "env-user")
	t.Setenv("IMAP_PASSWORD", "env-pass")

	cfg, err := Load(writeConfig(t, `
imap:
  tls: true
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.IMAP.Server != "imap.env.example" {
		t.Fatalf("expected IMAP server from env, got %q", cfg.IMAP.Server)
	}
	if cfg.IMAP.Username != "env-user" {
		t.Fatalf("expected IMAP username from env, got %q", cfg.IMAP.Username)
	}
	if cfg.IMAP.Password() != "env-pass" {
		t.Fatalf("expected IMAP password from env, got %q", cfg.IMAP.Password())
	}
}

func TestLoad_IMAPEnvOverridesYAML(t *testing.T) {
	t.Setenv("IMAP_SERVER", "imap.env.example")
	t.Setenv("IMAP_USERNAME", "env-user")
	t.Setenv("IMAP_PASSWORD", "env-pass")

	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.yaml.example"
  username: "yaml-user"
  tls: true
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.IMAP.Server != "imap.env.example" {
		t.Fatalf("expected env IMAP server to override yaml, got %q", cfg.IMAP.Server)
	}
	if cfg.IMAP.Username != "env-user" {
		t.Fatalf("expected env IMAP username to override yaml, got %q", cfg.IMAP.Username)
	}
	if cfg.IMAP.Password() != "env-pass" {
		t.Fatalf("expected IMAP password from env, got %q", cfg.IMAP.Password())
	}
}

func TestLoad_MissingIMAPEnvAllowedForDBOnlyCommands(t *testing.T) {
	t.Setenv("IMAP_SERVER", "imap.env.example")
	t.Setenv("IMAP_USERNAME", "env-user")

	cfg, err := Load(writeConfig(t, `
imap:
  tls: true
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IMAP.Password() != "" {
		t.Fatalf("expected empty IMAP password when IMAP_PASSWORD is not set, got %q", cfg.IMAP.Password())
	}
}

func TestLoad_NtfyTokenFromYAML(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  tls: true
notify:
  ntfy_url: "https://ntfy.sh/topic"
  ntfy_token: "yaml-token"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Notify.NtfyToken != "yaml-token" {
		t.Fatalf("expected ntfy token from yaml, got %q", cfg.Notify.NtfyToken)
	}
}

func TestLoad_NtfyTokenEnvOverridesYAML(t *testing.T) {
	t.Setenv("NTFY_TOKEN", "env-token")

	cfg, err := Load(writeConfig(t, `
imap:
  tls: true
notify:
  ntfy_url: "https://ntfy.sh/topic"
  ntfy_token: "yaml-token"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Notify.NtfyToken != "env-token" {
		t.Fatalf("expected ntfy token from env, got %q", cfg.Notify.NtfyToken)
	}
}

func TestLoad_NtfyURLEnvOverridesYAML(t *testing.T) {
	t.Setenv("NTFY_URL", "https://ntfy.sh/env-topic")

	cfg, err := Load(writeConfig(t, `
imap:
  tls: true
notify:
  ntfy_url: "https://ntfy.sh/yaml-topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Notify.NtfyURL != "https://ntfy.sh/env-topic" {
		t.Fatalf("expected ntfy url from env, got %q", cfg.Notify.NtfyURL)
	}
}

func TestLoad_IMAPTLSDefaultsToTrueWhenUnset(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.example.com"
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.IMAP.TLS {
		t.Fatal("expected imap.tls to default to true when unset")
	}
}

func TestLoad_IMAPTLSRespectsExplicitFalse(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.example.com"
  tls: false
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IMAP.TLS {
		t.Fatal("expected imap.tls=false from yaml to be preserved")
	}
}

func TestLoad_IMAPPortDefaultsTo993WhenUnset(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.example.com"
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IMAP.Port != 993 {
		t.Fatalf("expected imap.port to default to 993, got %d", cfg.IMAP.Port)
	}
}

func TestLoad_IMAPPortRespectsExplicitZero(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.example.com"
  port: 0
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IMAP.Port != 0 {
		t.Fatalf("expected imap.port=0 from yaml to be preserved, got %d", cfg.IMAP.Port)
	}
}

func TestLoad_IMAPFolderDefaultsToINBOXWhenUnset(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.example.com"
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IMAP.Folder != "INBOX" {
		t.Fatalf("expected imap.folder to default to INBOX, got %q", cfg.IMAP.Folder)
	}
}

func TestLoad_IMAPFolderRespectsExplicitEmpty(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
imap:
  server: "imap.example.com"
  folder: ""
notify:
  ntfy_url: "https://ntfy.sh/topic"
`))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.IMAP.Folder != "" {
		t.Fatalf("expected empty imap.folder from yaml to be preserved, got %q", cfg.IMAP.Folder)
	}
}
