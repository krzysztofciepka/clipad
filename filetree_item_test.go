package main

import (
	"os"
	"path/filepath"
	"testing"
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

func expandAll(node *TreeNode) {
	if node.IsDir {
		node.Expanded = true
		for _, child := range node.Children {
			expandAll(child)
		}
	}
}
