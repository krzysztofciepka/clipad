package main

import (
	"testing"
)

func TestFilterFiles_ExactMatch(t *testing.T) {
	files := []*TreeNode{
		{Name: "ideas.md", Path: "/vault/ideas.md"},
		{Name: "todo.md", Path: "/vault/todo.md"},
		{Name: "readme.md", Path: "/vault/readme.md"},
	}
	results := filterFiles(files, "todo")
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Name != "todo.md" {
		t.Errorf("first result = %q, want %q", results[0].Name, "todo.md")
	}
}

func TestFilterFiles_FuzzyMatch(t *testing.T) {
	files := []*TreeNode{
		{Name: "meeting-notes.md", Path: "/vault/meeting-notes.md"},
		{Name: "readme.md", Path: "/vault/readme.md"},
	}
	results := filterFiles(files, "mtn")
	if len(results) == 0 {
		t.Fatal("expected fuzzy match for 'mtn' -> 'meeting-notes.md'")
	}
}

func TestFilterFiles_EmptyQuery(t *testing.T) {
	files := []*TreeNode{
		{Name: "a.md", Path: "/vault/a.md"},
		{Name: "b.md", Path: "/vault/b.md"},
	}
	results := filterFiles(files, "")
	if len(results) != 2 {
		t.Errorf("empty query should return all files, got %d", len(results))
	}
}

func TestFilterFiles_NoMatch(t *testing.T) {
	files := []*TreeNode{
		{Name: "ideas.md", Path: "/vault/ideas.md"},
	}
	results := filterFiles(files, "zzzzz")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
