package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "dest.md")
	os.WriteFile(src, []byte("hello"), 0o644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("dest content = %q, want %q", string(data), "hello")
	}
}

func TestCopyFile_SrcMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nope.md"), filepath.Join(dir, "dst.md"))
	if err == nil {
		t.Error("expected error for missing source, got nil")
	}
}

func TestUniquePath_NoConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	got := uniquePath(path)
	if got != path {
		t.Errorf("uniquePath() = %q, want %q", got, path)
	}
}

func TestUniquePath_Conflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	os.WriteFile(path, []byte(""), 0o644)

	got := uniquePath(path)
	want := filepath.Join(dir, "note (1).md")
	if got != want {
		t.Errorf("uniquePath() = %q, want %q", got, want)
	}
}

func TestUniquePath_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	os.WriteFile(path, []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "note (1).md"), []byte(""), 0o644)

	got := uniquePath(path)
	want := filepath.Join(dir, "note (2).md")
	if got != want {
		t.Errorf("uniquePath() = %q, want %q", got, want)
	}
}
