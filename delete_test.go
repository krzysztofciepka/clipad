package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountTreeContents_Empty(t *testing.T) {
	dir := t.TempDir()

	files, folders, err := countTreeContents(dir)
	if err != nil {
		t.Fatalf("countTreeContents: %v", err)
	}
	if files != 0 || folders != 0 {
		t.Errorf("got (files=%d, folders=%d), want (0, 0)", files, folders)
	}
}

func TestCountTreeContents_FilesAndFolders(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "c.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "deep", "d.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, folders, err := countTreeContents(dir)
	if err != nil {
		t.Fatalf("countTreeContents: %v", err)
	}
	if files != 4 {
		t.Errorf("files = %d, want 4", files)
	}
	if folders != 2 {
		t.Errorf("folders = %d, want 2", folders)
	}
}

func TestCountTreeContents_ExcludesRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "only"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, folders, err := countTreeContents(dir)
	if err != nil {
		t.Fatalf("countTreeContents: %v", err)
	}
	if folders != 1 {
		t.Errorf("folders = %d, want 1 (root must be excluded)", folders)
	}
}

func TestCountTreeContents_MissingPathReturnsError(t *testing.T) {
	_, _, err := countTreeContents(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}
