package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCycleShortcutProvider_BasicWrap(t *testing.T) {
	available := []string{"openrouter", "opencode"}
	if got := cycleShortcutProvider("openrouter", available); got != "opencode" {
		t.Errorf("cycle(openrouter) = %q, want opencode", got)
	}
	if got := cycleShortcutProvider("opencode", available); got != "openrouter" {
		t.Errorf("cycle(opencode) = %q, want openrouter (wrap)", got)
	}
}

func TestCycleShortcutProvider_CurrentNotInList(t *testing.T) {
	available := []string{"openrouter", "opencode"}
	if got := cycleShortcutProvider("missing", available); got != "openrouter" {
		t.Errorf("cycle(missing) = %q, want openrouter (first available)", got)
	}
}

func TestCycleShortcutProvider_EmptyAvailable(t *testing.T) {
	if got := cycleShortcutProvider("openrouter", nil); got != "openrouter" {
		t.Errorf("cycle with no available = %q, want unchanged openrouter", got)
	}
}

func TestCycleShortcutProvider_SingleAvailable(t *testing.T) {
	available := []string{"openrouter"}
	if got := cycleShortcutProvider("openrouter", available); got != "openrouter" {
		t.Errorf("cycle with one available = %q, want unchanged openrouter", got)
	}
}

func TestAvailableShortcutProviders_BothConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openrouter.toml"), []byte("api_key='k'\nmodel='m'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.toml"), []byte("api_key='k'\nmodel='m'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	got := availableShortcutProviders(plugins)
	want := []string{"openrouter", "opencode"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAvailableShortcutProviders_NoneConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	got := availableShortcutProviders(plugins)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestAvailableShortcutProviders_PartialConfigDropped(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openrouter.toml"), []byte("api_key='k'\nmodel='m'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.toml"), []byte("api_key='k'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	got := availableShortcutProviders(plugins)
	if len(got) != 1 || got[0] != "openrouter" {
		t.Errorf("got %v, want [openrouter] (opencode has incomplete config)", got)
	}
}

func TestResolveShortcutProvider_KeepsRegisteredName(t *testing.T) {
	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	if got := resolveShortcutProvider("opencode", plugins); got != "opencode" {
		t.Errorf("resolveShortcutProvider(opencode) = %q, want opencode", got)
	}
}

func TestResolveShortcutProvider_FallsBackForRemovedProvider(t *testing.T) {
	plugins := []Plugin{&OpenRouterPlugin{}, &OpenCodePlugin{}}
	// Configs written before blackbox was removed still name it.
	if got := resolveShortcutProvider("blackbox", plugins); got != defaultAIShortcutProvider {
		t.Errorf("resolveShortcutProvider(blackbox) = %q, want %q", got, defaultAIShortcutProvider)
	}
	if got := resolveShortcutProvider("", plugins); got != defaultAIShortcutProvider {
		t.Errorf("resolveShortcutProvider(\"\") = %q, want %q", got, defaultAIShortcutProvider)
	}
}
