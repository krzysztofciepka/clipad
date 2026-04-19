package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCycleShortcutProvider_BasicWrap(t *testing.T) {
	available := []string{"blackbox", "openrouter"}
	if got := cycleShortcutProvider("blackbox", available); got != "openrouter" {
		t.Errorf("cycle(blackbox) = %q, want openrouter", got)
	}
	if got := cycleShortcutProvider("openrouter", available); got != "blackbox" {
		t.Errorf("cycle(openrouter) = %q, want blackbox (wrap)", got)
	}
}

func TestCycleShortcutProvider_CurrentNotInList(t *testing.T) {
	available := []string{"blackbox", "openrouter"}
	if got := cycleShortcutProvider("missing", available); got != "blackbox" {
		t.Errorf("cycle(missing) = %q, want blackbox (first available)", got)
	}
}

func TestCycleShortcutProvider_EmptyAvailable(t *testing.T) {
	if got := cycleShortcutProvider("blackbox", nil); got != "blackbox" {
		t.Errorf("cycle with no available = %q, want unchanged blackbox", got)
	}
}

func TestCycleShortcutProvider_SingleAvailable(t *testing.T) {
	available := []string{"blackbox"}
	if got := cycleShortcutProvider("blackbox", available); got != "blackbox" {
		t.Errorf("cycle with one available = %q, want unchanged blackbox", got)
	}
}

func TestAvailableShortcutProviders_BothConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad", "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blackbox.toml"), []byte("api_key='k'\nmodel='m'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openrouter.toml"), []byte("api_key='k'\nmodel='m'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plugins := []Plugin{&BlackboxPlugin{}, &OpenRouterPlugin{}}
	got := availableShortcutProviders(plugins)
	want := []string{"blackbox", "openrouter"}
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

	plugins := []Plugin{&BlackboxPlugin{}, &OpenRouterPlugin{}}
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
	if err := os.WriteFile(filepath.Join(dir, "blackbox.toml"), []byte("api_key='k'\nmodel='m'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openrouter.toml"), []byte("api_key='k'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plugins := []Plugin{&BlackboxPlugin{}, &OpenRouterPlugin{}}
	got := availableShortcutProviders(plugins)
	if len(got) != 1 || got[0] != "blackbox" {
		t.Errorf("got %v, want [blackbox] (openrouter has incomplete config)", got)
	}
}
