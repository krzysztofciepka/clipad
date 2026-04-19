package main

import (
	"testing"

	toml "github.com/pelletier/go-toml/v2"
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

func TestDefaultShortcutsEmbeddedTOMLParses(t *testing.T) {
	var cfg aiShortcutsConfig
	if err := toml.Unmarshal(defaultShortcutsTOML, &cfg); err != nil {
		t.Fatalf("embedded defaults failed to parse: %v", err)
	}
	if len(cfg.Shortcuts) != 23 {
		t.Fatalf("embedded defaults: want 23 shortcuts, got %d", len(cfg.Shortcuts))
	}

	want := []string{
		"prd",
		"userstory", "acceptance", "critique",
		"todos", "prioritize", "breakdown",
		"onboard", "explain",
		"tighten", "tldr", "outline", "questions", "examples", "diagram", "glossary", "risks",
		"bullets", "steps", "table", "headers", "fmtjson", "markdown",
	}
	for i, n := range want {
		if cfg.Shortcuts[i].Name != n {
			t.Errorf("shortcut %d: want name %q, got %q", i, n, cfg.Shortcuts[i].Name)
		}
		if cfg.Shortcuts[i].Prompt == "" {
			t.Errorf("shortcut %q: empty prompt", n)
		}
	}
}
