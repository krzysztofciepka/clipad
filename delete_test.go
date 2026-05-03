package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestCtrlD_OnEmptyFolder_EntersConfirmWithZeroCounts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	idx := m.tree.indexOfPath(filepath.Join(vault, "research"))
	if idx < 0 {
		t.Fatalf("research not in tree items")
	}
	m.tree.cursor = idx

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputConfirmDelete {
		t.Fatalf("inputMode = %v, want inputConfirmDelete", nm.inputMode)
	}
	if nm.deleteTarget != filepath.Join(vault, "research") {
		t.Errorf("deleteTarget = %q, want %q", nm.deleteTarget, filepath.Join(vault, "research"))
	}
	if nm.deleteCount.files != 0 || nm.deleteCount.folders != 0 {
		t.Errorf("deleteCount = %+v, want zero", nm.deleteCount)
	}
}

func TestCtrlD_OnNonEmptyFolder_CountsContents(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "sub", "b.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	idx := m.tree.indexOfPath(target)
	if idx < 0 {
		t.Fatalf("proj not in tree items")
	}
	m.tree.cursor = idx

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputConfirmDelete {
		t.Fatalf("inputMode = %v, want inputConfirmDelete", nm.inputMode)
	}
	if nm.deleteCount.files != 2 {
		t.Errorf("deleteCount.files = %d, want 2", nm.deleteCount.files)
	}
	if nm.deleteCount.folders != 1 {
		t.Errorf("deleteCount.folders = %d, want 1", nm.deleteCount.folders)
	}
}

func TestCtrlD_OnAddNoteRow_NoOp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = -1

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.deleteTarget != "" {
		t.Errorf("deleteTarget = %q, want empty", nm.deleteTarget)
	}
}

func TestStatusBarPrompt_FolderEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, "research"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.width = 80
	m.height = 24
	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "research"))

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	out := next.(model).View()
	if !strings.Contains(out, `Delete folder "research"? (y/n)`) {
		t.Errorf("View() missing empty-folder prompt; got:\n%s", out)
	}
}

func TestStatusBarPrompt_FolderNonEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(target, "sub", "b.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.width = 80
	m.height = 24
	m.tree.cursor = m.tree.indexOfPath(target)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	out := next.(model).View()
	if !strings.Contains(out, `Delete folder "proj" (2 files, 1 folders)? (y/n)`) {
		t.Errorf("View() missing non-empty folder prompt; got:\n%s", out)
	}
}

func TestStatusBarPrompt_FileUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	file := filepath.Join(vault, "note.md")
	os.WriteFile(file, []byte("hi"), 0o644)
	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.width = 80
	m.height = 24
	m.tree.cursor = m.tree.indexOfPath(file)

	next, _ := m.handleTreeKeys(tea.KeyMsg{Type: tea.KeyCtrlD})
	out := next.(model).View()
	if !strings.Contains(out, "Delete note.md? (y/n)") {
		t.Errorf("View() missing file prompt; got:\n%s", out)
	}
}

func TestHandleDeleteConfirm_FolderRemovesRecursively(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	if err := os.MkdirAll(filepath.Join(target, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(target, "sub", "b.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(target)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = target

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", target, err)
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.deleteTarget != "" {
		t.Errorf("deleteTarget = %q, want empty", nm.deleteTarget)
	}
}

func TestHandleDeleteConfirm_FolderClearsOpenFileInside(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	open := filepath.Join(target, "a.md")
	os.MkdirAll(target, 0o755)
	os.WriteFile(open, []byte("hello"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.openFile(open)
	if m.currentFile != open {
		t.Fatalf("openFile didn't take")
	}
	m.tree.cursor = m.tree.indexOfPath(target)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = target

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if nm.currentFile != "" {
		t.Errorf("currentFile = %q, want empty", nm.currentFile)
	}
	if nm.editor.Value() != "" {
		t.Errorf("editor value = %q, want empty", nm.editor.Value())
	}
	if nm.cleanContent != "" {
		t.Errorf("cleanContent = %q, want empty", nm.cleanContent)
	}
	if nm.tree.currentFile != "" {
		t.Errorf("tree.currentFile = %q, want empty", nm.tree.currentFile)
	}
}

func TestHandleDeleteConfirm_FileStillWorks(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	file := filepath.Join(vault, "note.md")
	os.WriteFile(file, []byte("hi"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(file)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = file

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err = %v", file, err)
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
}

func TestHandleDeleteConfirm_NCancels(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	target := filepath.Join(vault, "proj")
	os.MkdirAll(target, 0o755)
	os.WriteFile(filepath.Join(target, "a.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(target)
	m.inputMode = inputConfirmDelete
	m.deleteTarget = target

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm := next.(model)

	if _, err := os.Stat(target); err != nil {
		t.Errorf("folder unexpectedly removed on 'n': %v", err)
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.deleteTarget != "" {
		t.Errorf("deleteTarget = %q, want empty", nm.deleteTarget)
	}
}

func TestPathIsInside(t *testing.T) {
	cases := []struct {
		path string
		root string
		want bool
	}{
		{"/v/a", "/v/a", true},
		{"/v/a/x.md", "/v/a", true},
		{"/v/a/sub/x.md", "/v/a", true},
		{"/v/ab", "/v/a", false},
		{"/v/a", "/v/a/sub", false},
		{"/v/b", "/v/a", false},
	}
	for _, tc := range cases {
		if got := pathIsInside(tc.path, tc.root); got != tc.want {
			t.Errorf("pathIsInside(%q, %q) = %v, want %v", tc.path, tc.root, got, tc.want)
		}
	}
}

func TestHandleDeleteConfirm_CursorOnNextSibling(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	os.MkdirAll(filepath.Join(vault, "a"), 0o755)
	os.MkdirAll(filepath.Join(vault, "b"), 0o755)
	os.WriteFile(filepath.Join(vault, "b", "keep.md"), []byte("x"), 0o644)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "a"))
	m.inputMode = inputConfirmDelete
	m.deleteTarget = filepath.Join(vault, "a")

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if got := nm.tree.selectedNode(); got == nil || got.Path != filepath.Join(vault, "b") {
		var p string
		if got != nil {
			p = got.Path
		}
		t.Errorf("cursor on %q, want %q", p, filepath.Join(vault, "b"))
	}
}

func TestHandleDeleteConfirm_LastChildLandsOnParent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	os.MkdirAll(filepath.Join(vault, "foo", "a"), 0o755)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	fooNode := findNodeByPath(m.treeRoot, filepath.Join(vault, "foo"))
	if fooNode == nil {
		t.Fatal("foo not in tree")
	}
	fooNode.Expanded = true
	m.tree.rebuildItems()

	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "foo", "a"))
	m.inputMode = inputConfirmDelete
	m.deleteTarget = filepath.Join(vault, "foo", "a")

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if got := nm.tree.selectedNode(); got == nil || got.Path != filepath.Join(vault, "foo") {
		var p string
		if got != nil {
			p = got.Path
		}
		t.Errorf("cursor on %q, want %q (parent)", p, filepath.Join(vault, "foo"))
	}
}

func TestHandleDeleteConfirm_LastTopLevelLandsOnAddNoteRow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	os.MkdirAll(filepath.Join(vault, "only"), 0o755)

	m := newModel(vault, nil, "", "")
	m.refreshTree()
	m.tree.cursor = m.tree.indexOfPath(filepath.Join(vault, "only"))
	m.inputMode = inputConfirmDelete
	m.deleteTarget = filepath.Join(vault, "only")

	next, _ := m.handleDeleteConfirm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(model)

	if nm.tree.cursor != -1 {
		t.Errorf("cursor = %d, want -1 (Add note row)", nm.tree.cursor)
	}
}

func findNodeByPath(root *TreeNode, path string) *TreeNode {
	if root == nil {
		return nil
	}
	if root.Path == path {
		return root
	}
	for _, c := range root.Children {
		if hit := findNodeByPath(c, path); hit != nil {
			return hit
		}
	}
	return nil
}
