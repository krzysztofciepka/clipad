package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderTemplate_Variables(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	in := "d={{date}} t={{time}} y={{yesterday}} v={{vault}} c={{date:02 Jan 2006}}"
	want := "d=2026-05-25 t=14:30 y=2026-05-24 v=/tmp/vault c=25 May 2026"
	got := renderTemplate(in, now, "/tmp/vault")
	if got != want {
		t.Errorf("renderTemplate:\n got  %q\n want %q", got, want)
	}
}

func TestRenderTemplate_UnknownPlaceholdersUntouched(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	in := "{{foo}} {{date}} {{bar:x}} literal {{ }}"
	want := "{{foo}} 2026-05-25 {{bar:x}} literal {{ }}"
	got := renderTemplate(in, now, "/v")
	if got != want {
		t.Errorf("renderTemplate:\n got  %q\n want %q", got, want)
	}
}

func TestSeedDefaultTemplate_CreatesWhenAbsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := seedDefaultTemplate(); err != nil {
		t.Fatalf("seedDefaultTemplate: %v", err)
	}
	path := filepath.Join(templatesDir(), "daily.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("daily.md not created: %v", err)
	}
	if !strings.Contains(string(data), "{{date") {
		t.Errorf("seeded template missing date variable:\n%s", data)
	}
}

func TestSeedDefaultTemplate_DoesNotOverwrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(templatesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(templatesDir(), "daily.md")
	if err := os.WriteFile(path, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := seedDefaultTemplate(); err != nil {
		t.Fatalf("seedDefaultTemplate: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "custom" {
		t.Errorf("seed overwrote existing template: got %q", data)
	}
}
