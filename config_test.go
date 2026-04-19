package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestSaveAndLoadConfig_GitSync(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	now := time.Now().Truncate(time.Second)
	cfg := Config{
		Vault:     "/tmp/my-vault",
		GitRemote: "git@github.com:user/vault.git",
		LastSync:  &now,
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if loaded.GitRemote != cfg.GitRemote {
		t.Errorf("loaded.GitRemote = %q, want %q", loaded.GitRemote, cfg.GitRemote)
	}
	if loaded.LastSync == nil {
		t.Fatal("loaded.LastSync is nil, want non-nil")
	}
	if !loaded.LastSync.Equal(now) {
		t.Errorf("loaded.LastSync = %v, want %v", loaded.LastSync, now)
	}
}

func TestSaveAndLoadConfig_AIShortcutProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg := Config{Vault: "/tmp/my-vault", AIShortcutProvider: "openrouter"}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error: %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if loaded.AIShortcutProvider != "openrouter" {
		t.Errorf("loaded.AIShortcutProvider = %q, want %q", loaded.AIShortcutProvider, "openrouter")
	}
}

func TestLoadConfig_AIShortcutProviderDefault(t *testing.T) {
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
	if loaded.AIShortcutProvider != "blackbox" {
		t.Errorf("loaded.AIShortcutProvider = %q, want default %q", loaded.AIShortcutProvider, "blackbox")
	}
}

func TestSaveAndLoadConfig_GitSyncEmpty(t *testing.T) {
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
	if loaded.GitRemote != "" {
		t.Errorf("loaded.GitRemote = %q, want empty", loaded.GitRemote)
	}
	if loaded.LastSync != nil {
		t.Errorf("loaded.LastSync = %v, want nil", loaded.LastSync)
	}
}
