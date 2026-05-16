package main

import (
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestSaveAndLoadShortcuts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	shortcuts := []AIShortcut{
		{Name: "Fix grammar", Description: "Correct grammar errors", Prompt: "Fix grammar errors"},
		{Name: "Summarize", Description: "Short summary", Prompt: "Summarize this text"},
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
	if loaded[0].Description != "Correct grammar errors" {
		t.Errorf("first shortcut description = %q, want %q", loaded[0].Description, "Correct grammar errors")
	}
	if loaded[1].Prompt != "Summarize this text" {
		t.Errorf("second shortcut prompt = %q, want %q", loaded[1].Prompt, "Summarize this text")
	}
}

func TestLoadShortcuts_SeedsWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 23 {
		t.Fatalf("expected 23 seeded shortcuts, got %d", len(loaded))
	}
	if loaded[0].Name != "prd" {
		t.Errorf("first seeded shortcut: want %q, got %q", "prd", loaded[0].Name)
	}

	path := filepath.Join(tmpDir, "clipad", "ai_shortcuts.toml")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected seeded file at %s: %v", path, err)
	}
	if string(got) != string(defaultShortcutsTOML) {
		t.Errorf("seeded file content does not match embedded defaults")
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

func TestLoadShortcuts_DoesNotOverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("[[shortcuts]]\nname = 'mine'\nprompt = 'do my thing'\n")
	path := filepath.Join(dir, "ai_shortcuts.toml")
	if err := os.WriteFile(path, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("want 1 shortcut, got %d", len(loaded))
	}
	if loaded[0].Name != "mine" {
		t.Errorf("want name %q, got %q", "mine", loaded[0].Name)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Errorf("file was overwritten:\nwant: %q\ngot:  %q", custom, got)
	}
}

func TestLoadShortcuts_KeepsExplicitlyEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	dir := filepath.Join(tmpDir, "clipad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	empty := []byte("# user has no shortcuts\n")
	path := filepath.Join(dir, "ai_shortcuts.toml")
	if err := os.WriteFile(path, empty, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("want 0 shortcuts (file present, empty intent), got %d", len(loaded))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(empty) {
		t.Errorf("file was overwritten with defaults:\nwant: %q\ngot:  %q", empty, got)
	}
}

func TestResolveShortcutType(t *testing.T) {
	cases := []struct {
		name string
		in   AIShortcut
		want string
	}{
		{"explicit replace", AIShortcut{Name: "x", Type: "replace"}, "replace"},
		{"explicit review", AIShortcut{Name: "x", Type: "review"}, "review"},
		{"empty critique infers review", AIShortcut{Name: "critique"}, "review"},
		{"empty questions infers review", AIShortcut{Name: "questions"}, "review"},
		{"empty risks infers review", AIShortcut{Name: "risks"}, "review"},
		{"empty outline infers review", AIShortcut{Name: "outline"}, "review"},
		{"empty replace built-in", AIShortcut{Name: "tighten"}, "replace"},
		{"empty custom name", AIShortcut{Name: "my-custom"}, "replace"},
		{"unrecognised type string", AIShortcut{Name: "critique", Type: "bogus"}, "review"},
		{"unrecognised type custom name", AIShortcut{Name: "foo", Type: "bogus"}, "replace"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveShortcutType(c.in); got != c.want {
				t.Errorf("resolveShortcutType(%+v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestDefaultShortcuts_HaveResolvedTypes(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}

	wantReview := map[string]bool{"critique": true, "questions": true, "risks": true, "outline": true}
	seenReview := map[string]bool{}
	for _, s := range loaded {
		got := resolveShortcutType(s)
		if wantReview[s.Name] {
			if got != "review" {
				t.Errorf("%q: type = %q, want review", s.Name, got)
			}
			seenReview[s.Name] = true
		} else if got != "replace" {
			t.Errorf("%q: type = %q, want replace", s.Name, got)
		}
	}
	for name := range wantReview {
		if !seenReview[name] {
			t.Errorf("expected built-in %q not found in defaults", name)
		}
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
		if cfg.Shortcuts[i].Description == "" {
			t.Errorf("shortcut %q: empty description", n)
		}
	}
}
