package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	// Fetch to see if remote already has commits
	cmd := exec.Command("git", "-C", local, "fetch", "origin")
	cmd.Run()
	out, _ := exec.Command("git", "-C", local, "ls-remote", "--heads", remote).Output()
	if strings.Contains(string(out), "refs/heads/") {
		// Remote already has commits — check out from remote
		run("checkout", "-b", "master", "origin/master")
		run("branch", "--set-upstream-to=origin/master", "master")
	} else {
		// Fresh remote — create initial commit and push
		os.WriteFile(filepath.Join(local, ".gitkeep"), []byte(""), 0o644)
		run("add", "-A")
		run("commit", "-m", "initial")
		run("push", "-u", "origin", "HEAD")
	}
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

func TestRunGitSync_RemoteDelete_NoConflict(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)

	// Seed a file via "other" machine, then delete it via "other".
	other := initLocalWithRemote(t, remote)
	os.WriteFile(filepath.Join(other, "doomed.md"), []byte("bye"), 0o644)
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "add doomed").Run()
	exec.Command("git", "-C", other, "push").Run()

	// Pull the seed into local so it has a copy.
	exec.Command("git", "-C", local, "pull", "--rebase", "origin", "HEAD").Run()
	if _, err := os.Stat(filepath.Join(local, "doomed.md")); err != nil {
		t.Fatalf("seed missing locally: %v", err)
	}

	// Now "other" deletes it and pushes.
	os.Remove(filepath.Join(other, "doomed.md"))
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "delete doomed").Run()
	exec.Command("git", "-C", other, "push").Run()

	// Local syncs — no local edits, just pulling the deletion.
	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if !msg.pulled {
		t.Error("pulled = false, want true")
	}
	if _, err := os.Stat(filepath.Join(local, "doomed.md")); !os.IsNotExist(err) {
		t.Errorf("doomed.md should be gone locally; stat err=%v", err)
	}
	// No .sync-conflict siblings should exist.
	matches, _ := filepath.Glob(filepath.Join(local, "*sync-conflict*"))
	if len(matches) != 0 {
		t.Errorf("unexpected sync-conflict files: %v", matches)
	}
}

func TestRunGitSync_Conflict(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)
	other := initLocalWithRemote(t, remote)

	// Both machines edit the same file
	os.WriteFile(filepath.Join(other, ".gitkeep"), []byte("other content"), 0o644)
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "other edit").Run()
	exec.Command("git", "-C", other, "push").Run()

	os.WriteFile(filepath.Join(local, ".gitkeep"), []byte("local content"), 0o644)
	exec.Command("git", "-C", local, "add", "-A").Run()
	exec.Command("git", "-C", local, "commit", "-m", "local edit").Run()

	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)

	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if !msg.pushed {
		t.Error("pushed = false, want true after conflict resolution")
	}

	// The sync-conflict file should exist with the remote version
	conflictPath := filepath.Join(local, ".sync-conflict.gitkeep")
	data, err := os.ReadFile(conflictPath)
	if err != nil {
		t.Fatalf(".sync-conflict.gitkeep not found: %v", err)
	}
	if string(data) != "other content" {
		t.Errorf("conflict file content = %q, want %q", string(data), "other content")
	}

	// Local version should be preserved
	localData, _ := os.ReadFile(filepath.Join(local, ".gitkeep"))
	if string(localData) != "local content" {
		t.Errorf("local file content = %q, want %q", string(localData), "local content")
	}
}

func TestRunGitSync_ConflictWithExtension(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)
	other := initLocalWithRemote(t, remote)

	// Both machines edit a .md file
	os.WriteFile(filepath.Join(other, "notes.md"), []byte("remote notes"), 0o644)
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "other notes").Run()
	exec.Command("git", "-C", other, "push").Run()

	os.WriteFile(filepath.Join(local, "notes.md"), []byte("local notes"), 0o644)
	exec.Command("git", "-C", local, "add", "-A").Run()
	exec.Command("git", "-C", local, "commit", "-m", "local notes").Run()

	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)

	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}

	// Conflict file should be notes.sync-conflict.md
	conflictPath := filepath.Join(local, "notes.sync-conflict.md")
	if _, err := os.Stat(conflictPath); err != nil {
		t.Fatalf("notes.sync-conflict.md not found: %v", err)
	}
}

func TestSyncConflictName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"notes.md", "notes.sync-conflict.md"},
		{"image.png", "image.sync-conflict.png"},
		{"Makefile", "Makefile.sync-conflict"},
		{"archive.tar.gz", "archive.tar.sync-conflict.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := syncConflictName(tt.input)
			if got != tt.want {
				t.Errorf("syncConflictName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTriggerManualGitSync_SkipsIfAlreadyRunning(t *testing.T) {
	m := newTestModel(t)
	m.gitSyncRunning = true
	next, cmd := m.triggerManualGitSync()
	if cmd != nil {
		t.Error("expected nil cmd when already running")
	}
	nm := next.(model)
	if !nm.gitSyncRunning {
		t.Error("gitSyncRunning should remain true")
	}
}

func TestTriggerManualGitSync_ConfigErrorSetsErrMsg(t *testing.T) {
	m := newTestModel(t)
	next, cmd := m.triggerManualGitSync()
	if cmd != nil {
		t.Error("expected nil cmd on config error")
	}
	nm := next.(model)
	if nm.errMsg == "" {
		t.Error("errMsg should be set on config error")
	}
}

func TestTriggerManualGitSync_NoRemotePromptsForURL(t *testing.T) {
	m := newTestModel(t)
	xdg := os.Getenv("XDG_CONFIG_HOME")
	cfgDir := filepath.Join(xdg, "clipad")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfgDir, "config.toml"),
		[]byte(`vault = "/tmp/test"`+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}

	next, _ := m.triggerManualGitSync()
	nm := next.(model)
	if nm.inputMode != inputGitRemote {
		t.Errorf("inputMode = %v, want inputGitRemote", nm.inputMode)
	}
}

func TestTriggerManualGitSync_BypassesLastSyncGuard(t *testing.T) {
	m := newTestModel(t)
	remote := initBareRemote(t)
	m.vault = initLocalWithRemote(t, remote)

	xdg := os.Getenv("XDG_CONFIG_HOME")
	cfgDir := filepath.Join(xdg, "clipad")
	os.MkdirAll(cfgDir, 0o755)
	recent := time.Now().Format(time.RFC3339)
	cfg := `vault = "` + m.vault + `"` + "\n" +
		`git_remote = "` + remote + `"` + "\n" +
		`last_sync = "` + recent + `"` + "\n"
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(cfg), 0o644)

	next, cmd := m.triggerManualGitSync()
	nm := next.(model)
	if !nm.gitSyncRunning {
		t.Error("gitSyncRunning should be true after manual trigger")
	}
	if cmd == nil {
		t.Error("expected non-nil sync cmd")
	}
}
