package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPath_XDGSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := configPath()
	want := "/tmp/test-xdg/clipad/config.toml"
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestConfigPath_XDGUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	got := configPath()
	want := filepath.Join(home, ".config", "clipad", "config.toml")
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := loadConfig()
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg := Config{Vault: "/tmp/my-vault"}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if loaded.Vault != cfg.Vault {
		t.Errorf("loaded.Vault = %q, want %q", loaded.Vault, cfg.Vault)
	}
}
