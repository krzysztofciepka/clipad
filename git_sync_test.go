package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func initBareRemote(t *testing.T) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s: %v", out, err)
	}
	return remote
}

func initLocalWithRemote(t *testing.T, remote string) string {
	t.Helper()
	local := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = local
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init")
	run("remote", "add", "origin", remote)
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	// Create initial commit so we have a branch
	os.WriteFile(filepath.Join(local, ".gitkeep"), []byte(""), 0o644)
	run("add", "-A")
	run("commit", "-m", "initial")
	run("push", "-u", "origin", "HEAD")
	return local
}

func TestRunGitSync_NoChanges(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)

	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)

	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if msg.pushed {
		t.Error("pushed = true, want false (no changes)")
	}
}

func TestRunGitSync_LocalChanges(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)

	// Create a new file in the local vault
	os.WriteFile(filepath.Join(local, "note.md"), []byte("# Hello"), 0o644)

	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)

	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if !msg.pushed {
		t.Error("pushed = false, want true")
	}

	// Verify the commit exists in the remote
	out, _ := gitCmd(local, "log", "--oneline", "-1")
	if !strings.Contains(out, "clipad backup:") {
		t.Errorf("commit message = %q, want 'clipad backup:' prefix", out)
	}
}

func TestRunGitSync_RemoteChanges(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)

	// Simulate another machine pushing a file by cloning and pushing
	other := initLocalWithRemote(t, remote)
	os.WriteFile(filepath.Join(other, "from-other.md"), []byte("# Other"), 0o644)
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "other machine").Run()
	exec.Command("git", "-C", other, "push").Run()

	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)

	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if !msg.pulled {
		t.Error("pulled = false, want true")
	}

	// Verify the file arrived locally
	if _, err := os.Stat(filepath.Join(local, "from-other.md")); err != nil {
		t.Error("from-other.md not found in local after pull")
	}
}
