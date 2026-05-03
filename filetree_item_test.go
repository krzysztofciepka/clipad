package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupTestVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "notes", "daily"), 0o755)
	os.MkdirAll(filepath.Join(dir, "projects"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, "notes", "daily", "jan.md"), []byte("# Jan"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "ideas.md"), []byte("# Ideas"), 0o644)
	os.WriteFile(filepath.Join(dir, "projects", "todo.md"), []byte("# Todo"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Readme"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.md"), []byte("# Secret"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "ignore.txt"), []byte("not markdown"), 0o644)
	return dir
}

func TestBuildTree(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	if len(root.Children) != 3 {
		t.Errorf("root has %d children, want 3", len(root.Children))
	}
}

func TestBuildTree_SortOrder(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	if root.Children[0].Name != "notes" {
		t.Errorf("first child = %q, want %q", root.Children[0].Name, "notes")
	}
	if root.Children[1].Name != "projects" {
		t.Errorf("second child = %q, want %q", root.Children[1].Name, "projects")
	}
	if root.Children[2].Name != "readme.md" {
		t.Errorf("third child = %q, want %q", root.Children[2].Name, "readme.md")
	}
}

func TestBuildTree_HiddenExcluded(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	for _, child := range root.Children {
		if child.Name == ".hidden" {
			t.Error("hidden directory should be excluded")
		}
	}
}

func TestFlattenTree(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	expandAll(root)
	items := flattenTree(root, 0)
	if len(items) != 7 {
		names := make([]string, len(items))
		for i, item := range items {
			names[i] = item.Node.Name
		}
		t.Errorf("flattenTree() returned %d items %v, want 7", len(items), names)
	}
}

func TestCollectFiles(t *testing.T) {
	dir := setupTestVault(t)
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	files := collectFiles(root)
	if len(files) != 4 {
		t.Errorf("collectFiles() returned %d files, want 4", len(files))
	}
}

func TestBuildTree_EmptyFolderRenders(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}

	var found bool
	for _, child := range root.Children {
		if child.Name == "empty" {
			found = true
			if !child.IsDir {
				t.Errorf("empty entry IsDir = false, want true")
			}
			if len(child.Children) != 0 {
				t.Errorf("empty.Children len = %d, want 0", len(child.Children))
			}
		}
	}
	if !found {
		names := make([]string, len(root.Children))
		for i, c := range root.Children {
			names[i] = c.Name
		}
		t.Errorf("empty folder not in tree; got %v", names)
	}
}

func TestBuildTree_AllEmptyFoldersStillRender(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	if len(root.Children) != 3 {
		names := make([]string, len(root.Children))
		for i, c := range root.Children {
			names[i] = c.Name
		}
		t.Errorf("got %d children %v, want 3", len(root.Children), names)
	}
}

func TestBuildTree_HiddenDirStillExcluded_EmptyOrNot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := buildTree(dir)
	if err != nil {
		t.Fatalf("buildTree() error: %v", err)
	}
	for _, child := range root.Children {
		if child.Name == ".hidden" {
			t.Error(".hidden directory should be excluded even when empty")
		}
	}
}

func TestHandleNewFolder_DoesNotSeedUntitledMd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "", "")
	m.inputMode = inputNewFolder
	m.newFolderInput.SetValue("research")

	next, _ := m.handleNewFolder(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(model)

	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}

	folder := filepath.Join(vault, "research")
	st, err := os.Stat(folder)
	if err != nil || !st.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("new folder is not empty: %v", names)
	}
}

func expandAll(node *TreeNode) {
	if node.IsDir {
		node.Expanded = true
		for _, child := range node.Children {
			expandAll(child)
		}
	}
}
