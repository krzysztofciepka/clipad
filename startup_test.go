package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestPrepareStartup_CreatesFileAndParents(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "note.md")
	a := startupAction{kind: startupOpenFile, path: target, needsCreate: true}
	if err := prepareStartup(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestPrepareStartup_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "newdir")
	a := startupAction{kind: startupNewNoteInDir, path: target, needsMkdir: true}
	if err := prepareStartup(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Errorf("dir not created: err=%v", err)
	}
}

func TestPrepareStartup_ExistingFile_NoOp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.md")
	if err := os.WriteFile(file, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := startupAction{kind: startupOpenFile, path: file} // no needsCreate
	if err := prepareStartup(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "keep" {
		t.Errorf("file content changed: %q", data)
	}
}

func newStartupTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	return newModel(t.TempDir(), nil, "", "")
}

func TestApplyStartup_OpenFileEdit(t *testing.T) {
	m := newStartupTestModel(t)
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("body text"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.startup = startupAction{kind: startupOpenFile, path: file, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.currentFile != file {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, file)
	}
	if nm.editor.Value() != "body text" {
		t.Errorf("editor content = %q, want %q", nm.editor.Value(), "body text")
	}
	if nm.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", nm.editorMode)
	}
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", nm.activePanel)
	}
	if !nm.treeHidden {
		t.Error("treeHidden should be true")
	}
	if !nm.startupDone {
		t.Error("startupDone should be true")
	}
}

func TestApplyStartup_NewNote_TreeVisible(t *testing.T) {
	m := newStartupTestModel(t)
	// --new resolves to startupNewNote with hideTree=false (vault root).
	m.startup = startupAction{kind: startupNewNote, path: m.vault, hideTree: false}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.newNoteDir != m.vault {
		t.Errorf("newNoteDir = %q, want %q", nm.newNoteDir, m.vault)
	}
	if nm.treeHidden {
		t.Error("treeHidden should be false for --new")
	}
	if nm.treeWidth == 0 {
		t.Error("treeWidth should be > 0 when the tree is visible")
	}
}

func TestApplyStartup_OpenFilePreview(t *testing.T) {
	m := newStartupTestModel(t)
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("# Heading\n\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.startup = startupAction{kind: startupOpenFile, path: file, preview: true, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.editorMode != modePreview {
		t.Errorf("editorMode = %v, want modePreview", nm.editorMode)
	}
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", nm.activePanel)
	}
	if nm.currentFile != file {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, file)
	}
	// The preview must be Markdown-rendered (glamour emits ANSI styling),
	// not raw text — raw wordWrap output would never contain an escape code.
	if !strings.Contains(nm.preview.View(), "\x1b[") {
		t.Errorf("preview is not Markdown-rendered (no ANSI styling found)")
	}
}

func TestApplyStartup_NewNoteInDir(t *testing.T) {
	m := newStartupTestModel(t)
	dir := t.TempDir()
	m.startup = startupAction{kind: startupNewNoteInDir, path: dir, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	if nm.newNoteDir != dir {
		t.Errorf("newNoteDir = %q, want %q", nm.newNoteDir, dir)
	}
	if nm.currentFile != "" {
		t.Errorf("currentFile = %q, want empty", nm.currentFile)
	}
	if nm.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", nm.editorMode)
	}
	if nm.editor.Value() != "" {
		t.Errorf("editor not empty: %q", nm.editor.Value())
	}
}

func TestApplyStartup_RunsOnce(t *testing.T) {
	m := newStartupTestModel(t)
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.startup = startupAction{kind: startupOpenFile, path: file, hideTree: true}

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	nm := next.(model)
	nm.editor.SetValue("user typed")
	next2, _ := nm.Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	nm2 := next2.(model)
	if nm2.editor.Value() != "user typed" {
		t.Errorf("startup re-applied on second resize; editor = %q", nm2.editor.Value())
	}
}
