package main

import (
	"testing"
)

func TestSaveAndLoadShortcuts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	shortcuts := []AIShortcut{
		{Name: "Fix grammar", Prompt: "Fix grammar errors"},
		{Name: "Summarize", Prompt: "Summarize this text"},
	}
	if err := saveShortcuts(shortcuts); err != nil {
		t.Fatalf("saveShortcuts() error: %v", err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d shortcuts, want 2", len(loaded))
	}
	if loaded[0].Name != "Fix grammar" {
		t.Errorf("first shortcut name = %q, want %q", loaded[0].Name, "Fix grammar")
	}
	if loaded[1].Prompt != "Summarize this text" {
		t.Errorf("second shortcut prompt = %q, want %q", loaded[1].Prompt, "Summarize this text")
	}
}

func TestLoadShortcuts_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() should not error for missing file: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d shortcuts", len(loaded))
	}
}

func TestShortcutsPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := shortcutsPath()
	want := "/tmp/test-xdg/clipad/ai_shortcuts.toml"
	if got != want {
		t.Errorf("shortcutsPath() = %q, want %q", got, want)
	}
}
