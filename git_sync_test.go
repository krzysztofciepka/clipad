package main

import (
	"os/exec"
	"testing"
)

func TestGitCmd(t *testing.T) {
	dir := t.TempDir()
	// init a git repo so git commands work
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	out, err := gitCmd(dir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("gitCmd() error: %v", err)
	}
	// empty repo, no files — output should be empty
	if out != "" {
		t.Errorf("gitCmd() = %q, want empty", out)
	}
}

func TestIsGitRepo(t *testing.T) {
	t.Run("not a repo", func(t *testing.T) {
		dir := t.TempDir()
		if isGitRepo(dir) {
			t.Error("isGitRepo() = true for non-repo dir")
		}
	})

	t.Run("is a repo", func(t *testing.T) {
		dir := t.TempDir()
		exec.Command("git", "init", dir).Run()
		if !isGitRepo(dir) {
			t.Error("isGitRepo() = false for git repo")
		}
	})
}
