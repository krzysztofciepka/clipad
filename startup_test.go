package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStartup_ExistingFile_Edit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveStartup(false, false, file, dir, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.kind != startupOpenFile || got.path != file || got.preview || !got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_ExistingFile_Preview(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := resolveStartup(true, false, file, dir, dir)
	if got.kind != startupOpenFile || !got.preview || !got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_RelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rel.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := resolveStartup(false, false, "rel.md", dir, dir)
	if got.kind != startupOpenFile || got.path != filepath.Join(dir, "rel.md") {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_ExistingDir_NewNote(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ := resolveStartup(false, false, sub, dir, dir)
	if got.kind != startupNewNoteInDir || got.path != sub || !got.hideTree || got.needsMkdir {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_MissingFile_NeedsCreate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new", "deep.md")
	got, _ := resolveStartup(false, false, target, dir, dir)
	if got.kind != startupOpenFile || !got.needsCreate || !got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_MissingDirTrailingSlash_NeedsMkdir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "newdir") + "/"
	got, _ := resolveStartup(false, false, target, dir, dir)
	if got.kind != startupNewNoteInDir || !got.needsMkdir {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "note.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveStartup(false, false, "~/note.md", "/tmp", "/vault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "note.md")
	if got.kind != startupOpenFile || got.path != want {
		t.Errorf("got %+v, want path %s", got, want)
	}
}

func TestResolveStartup_NewFlag_VaultRoot(t *testing.T) {
	vault := t.TempDir()
	got, _ := resolveStartup(false, true, "", "/tmp", vault)
	if got.kind != startupNewNote || got.path != vault || got.hideTree {
		t.Errorf("got %+v", got)
	}
}

func TestResolveStartup_PreviewNoPath_Errors(t *testing.T) {
	if _, err := resolveStartup(true, false, "", "/tmp", "/vault"); err == nil {
		t.Error("expected error for -p with no path")
	}
}

func TestResolveStartup_NoArgs_None(t *testing.T) {
	got, err := resolveStartup(false, false, "", "/tmp", "/vault")
	if err != nil || got.kind != startupNone {
		t.Errorf("got %+v err %v", got, err)
	}
}
