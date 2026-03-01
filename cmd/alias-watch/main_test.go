package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lorenzbischof/alias-watch/internal/db"
)

func TestImportCommand_DBOnlyConfig(t *testing.T) {
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "data.db")
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := []byte(
		"db:\n" +
			"  path: \"" + dbPath + "\"\n" +
			"notify:\n" +
			"  ntfy_url: \"http://127.0.0.1/topic\"\n",
	)
	if err := os.WriteFile(configPath, configContent, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	csvPath := filepath.Join(tmpDir, "aliases.csv")
	csvContent := []byte(
		"\"id\",\"email\",\"active\",\"description\"\n" +
			"\"alias-1\",\"first@example.com\",\"TRUE\",\"First Alias\"\n" +
			"\"alias-2\",\"second@example.com\",\"\",\"\"\n",
	)
	if err := os.WriteFile(csvPath, csvContent, 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	prevCfgPath := cfgPath
	cfgPath = configPath
	t.Cleanup(func() {
		cfgPath = prevCfgPath
	})

	cmd := cmdImport()
	cmd.SetArgs([]string{csvPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute import command: %v", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	aliases, err := store.AllAliases()
	if err != nil {
		t.Fatalf("read aliases: %v", err)
	}
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}

	got := make(map[string]db.Alias, len(aliases))
	for _, a := range aliases {
		got[a.Email] = a
	}

	first := got["first@example.com"]
	if first.AddyID != "alias-1" || !first.Active || first.Title != "First Alias" {
		t.Fatalf("unexpected first alias: %+v", first)
	}

	second := got["second@example.com"]
	if second.AddyID != "alias-2" || second.Active || second.Title != "" {
		t.Fatalf("unexpected second alias: %+v", second)
	}
}

func TestImportCommand_FromStdin(t *testing.T) {
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "data.db")
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := []byte(
		"db:\n" +
			"  path: \"" + dbPath + "\"\n" +
			"notify:\n" +
			"  ntfy_url: \"http://127.0.0.1/topic\"\n",
	)
	if err := os.WriteFile(configPath, configContent, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	csvContent := "\"id\",\"email\",\"active\",\"description\"\n" +
		"\"alias-stdin\",\"stdin@example.com\",\"TRUE\",\"From stdin\"\n"

	prevCfgPath := cfgPath
	cfgPath = configPath
	t.Cleanup(func() {
		cfgPath = prevCfgPath
	})

	cmd := cmdImport()
	cmd.SetArgs([]string{"-"})
	cmd.SetIn(strings.NewReader(csvContent))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute import command: %v", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	aliases, err := store.AllAliases()
	if err != nil {
		t.Fatalf("read aliases: %v", err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].Email != "stdin@example.com" {
		t.Fatalf("unexpected alias email: %q", aliases[0].Email)
	}
}

func TestLearnCommand_RequiresIMAPConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "data.db")
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("db:\n  path: \""+dbPath+"\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevCfgPath := cfgPath
	cfgPath = configPath
	t.Cleanup(func() {
		cfgPath = prevCfgPath
	})

	cmd := cmdLearn()
	cmd.SetArgs(nil)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected learn to fail without IMAP config")
	}
	if !strings.Contains(err.Error(), "missing IMAP server") {
		t.Fatalf("unexpected error: %v", err)
	}
}
