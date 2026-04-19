package main

import (
	"testing"
)

func TestPluginConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := pluginConfigPath("blackbox")
	want := "/tmp/test-xdg/clipad/plugins/blackbox.toml"
	if got != want {
		t.Errorf("pluginConfigPath() = %q, want %q", got, want)
	}
}

func TestSaveAndLoadPluginConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	values := map[string]string{
		"api_key": "sk-test-123",
		"model":   "openai/gpt-4o",
	}
	if err := savePluginConfig("testplugin", values); err != nil {
		t.Fatalf("savePluginConfig() error: %v", err)
	}

	loaded, err := loadPluginConfig("testplugin")
	if err != nil {
		t.Fatalf("loadPluginConfig() error: %v", err)
	}
	if loaded["api_key"] != "sk-test-123" {
		t.Errorf("api_key = %q, want %q", loaded["api_key"], "sk-test-123")
	}
	if loaded["model"] != "openai/gpt-4o" {
		t.Errorf("model = %q, want %q", loaded["model"], "openai/gpt-4o")
	}
}

func TestLoadPluginConfig_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	_, err := loadPluginConfig("nonexistent")
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestPluginConfigComplete(t *testing.T) {
	fields := []ConfigField{
		{Key: "api_key", Label: "API Key"},
		{Key: "model", Label: "Model"},
	}

	complete := map[string]string{"api_key": "key", "model": "m"}
	if !pluginConfigComplete(fields, complete) {
		t.Error("expected complete config to return true")
	}

	partial := map[string]string{"api_key": "key"}
	if pluginConfigComplete(fields, partial) {
		t.Error("expected partial config to return false")
	}

	empty := map[string]string{}
	if pluginConfigComplete(fields, empty) {
		t.Error("expected empty config to return false")
	}
}
