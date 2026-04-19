package main

import (
	"os"
	"path/filepath"
	"testing"
)

func newRenameTestModel(t *testing.T, vault string) model {
	t.Helper()
	m := newModel(vault, nil, "")
	return m
}

func TestDoRename_FileReappendsExtension(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.md")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("new"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	want := filepath.Join(dir, "new.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected %s to exist: %v", want, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected %s to be gone, err=%v", src, err)
	}
}

func TestDoRename_FolderRewritesOpenFilePath(t *testing.T) {
	dir := t.TempDir()
	oldFolder := filepath.Join(dir, "oldfolder")
	if err := os.MkdirAll(oldFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	openFile := filepath.Join(oldFolder, "note.md")
	if err := os.WriteFile(openFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newRenameTestModel(t, dir)
	m.currentFile = openFile
	m.tree.currentFile = openFile
	m.renameTarget = oldFolder
	m.renameIsDir = true

	if err := m.doRename("newfolder"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	wantOpen := filepath.Join(dir, "newfolder", "note.md")
	if m.currentFile != wantOpen {
		t.Errorf("currentFile = %q, want %q", m.currentFile, wantOpen)
	}
	if m.tree.currentFile != wantOpen {
		t.Errorf("tree.currentFile = %q, want %q", m.tree.currentFile, wantOpen)
	}
}

func TestDoRename_RejectsPathSeparator(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	err := m.doRename("sub/b")
	if err == nil {
		t.Fatal("expected error for path separator, got nil")
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("source should still exist, statErr=%v", statErr)
	}
}

func TestDoRename_RejectsExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	dst := filepath.Join(dir, "b.md")
	os.WriteFile(src, []byte("x"), 0o644)
	os.WriteFile(dst, []byte("y"), 0o644)
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	err := m.doRename("b")
	if err == nil {
		t.Fatal("expected error for existing target, got nil")
	}
	data, _ := os.ReadFile(src)
	if string(data) != "x" {
		t.Errorf("source clobbered: %q", data)
	}
}

func TestDoRename_UpdatesOpenFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "open.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.currentFile = src
	m.tree.currentFile = src
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("renamed"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	want := filepath.Join(dir, "renamed.md")
	if m.currentFile != want {
		t.Errorf("currentFile = %q, want %q", m.currentFile, want)
	}
	if m.tree.currentFile != want {
		t.Errorf("tree.currentFile = %q, want %q", m.tree.currentFile, want)
	}
}

func TestDoRename_ClearsClipboardOnMatch(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.fileClip = fileClipboard{path: src, op: clipCut}
	m.tree.cutPath = src
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("b"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	if !m.fileClip.empty() {
		t.Errorf("fileClip should be cleared, got %+v", m.fileClip)
	}
	if m.tree.cutPath != "" {
		t.Errorf("cutPath should be cleared, got %q", m.tree.cutPath)
	}
}

func TestDoRename_ClearsClipboardOnFolderContains(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "f")
	os.MkdirAll(folder, 0o755)
	clip := filepath.Join(folder, "a.md")
	os.WriteFile(clip, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.fileClip = fileClipboard{path: clip, op: clipCopy}
	m.tree.cutPath = clip
	m.renameTarget = folder
	m.renameIsDir = true

	if err := m.doRename("g"); err != nil {
		t.Fatalf("doRename: %v", err)
	}

	if !m.fileClip.empty() {
		t.Errorf("fileClip should be cleared, got %+v", m.fileClip)
	}
	if m.tree.cutPath != "" {
		t.Errorf("cutPath should be cleared, got %q", m.tree.cutPath)
	}
}

func TestDoRename_NoOpSameName(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	os.WriteFile(src, []byte("x"), 0o644)
	m := newRenameTestModel(t, dir)
	m.renameTarget = src
	m.renameIsDir = false

	if err := m.doRename("a"); err != nil {
		t.Fatalf("doRename: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should still exist: %v", err)
	}
}
