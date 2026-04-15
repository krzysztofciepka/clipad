package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type gitSyncCheckMsg struct{}

type gitSyncResultMsg struct {
	pulled  bool
	pushed  bool
	pushErr error
	err     error
}

type gitSyncFadeMsg struct{}

func gitSyncCheck() tea.Cmd {
	return tea.Tick(30*time.Minute, func(time.Time) tea.Msg {
		return gitSyncCheckMsg{}
	})
}

func gitSyncCheckImmediate() tea.Cmd {
	return func() tea.Msg {
		return gitSyncCheckMsg{}
	}
}

func gitSyncFadeTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return gitSyncFadeMsg{}
	})
}

// gitCmd runs a git command in the given directory and returns trimmed stdout.
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// isGitRepo checks if the directory is inside a git repository.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func runGitSync(vault, remote string) tea.Cmd {
	return func() tea.Msg {
		var pulled, pushed bool

		// Initialize repo if needed
		if !isGitRepo(vault) {
			if _, err := gitCmd(vault, "init"); err != nil {
				return gitSyncResultMsg{err: fmt.Errorf("git init: %w", err)}
			}
			if _, err := gitCmd(vault, "remote", "add", "origin", remote); err != nil {
				return gitSyncResultMsg{err: fmt.Errorf("git remote add: %w", err)}
			}
		}

		// Fetch
		gitCmd(vault, "fetch", "origin")

		// If no local commits yet, make an initial commit and push
		localHead, localErr := gitCmd(vault, "rev-parse", "HEAD")
		if localErr != nil {
			if _, err := gitCmd(vault, "add", "-A"); err != nil {
				return gitSyncResultMsg{err: fmt.Errorf("git add: %w", err)}
			}
			if _, err := gitCmd(vault, "commit", "-m", "clipad: initial backup"); err != nil {
				return gitSyncResultMsg{err: fmt.Errorf("git commit: %w", err)}
			}
			_, pushErr := gitCmd(vault, "push", "-u", "origin", "HEAD")
			return gitSyncResultMsg{pushed: true, pushErr: pushErr}
		}

		// Check if remote has commits we don't have
		remoteHead, _ := gitCmd(vault, "rev-parse", "origin/HEAD")

		// Pull (rebase) if remote has changes
		if remoteHead != "" && localHead != remoteHead {
			out, err := gitCmd(vault, "pull", "--rebase", "origin", "HEAD")
			if err != nil {
				// Try with --allow-unrelated-histories
				if strings.Contains(out, "unrelated histories") {
					_, err = gitCmd(vault, "pull", "--rebase", "--allow-unrelated-histories", "origin", "HEAD")
				}
				if err != nil {
					// Conflict: abort rebase, save remote versions as .sync-conflict files
					return handleSyncConflict(vault)
				}
			}
			afterPull, _ := gitCmd(vault, "rev-parse", "HEAD")
			if afterPull != localHead {
				pulled = true
			}
		}

		// Stage all changes
		if _, err := gitCmd(vault, "add", "-A"); err != nil {
			return gitSyncResultMsg{err: fmt.Errorf("git add: %w", err)}
		}

		// Check if there are staged changes
		_, err := gitCmd(vault, "diff", "--cached", "--quiet")
		if err != nil {
			// There are changes to commit
			timestamp := time.Now().Format("2006-01-02 15:04")
			msg := fmt.Sprintf("clipad backup: %s", timestamp)
			if _, err := gitCmd(vault, "commit", "-m", msg); err != nil {
				return gitSyncResultMsg{err: fmt.Errorf("git commit: %w", err)}
			}

			// Push
			_, pushErr := gitCmd(vault, "push", "-u", "origin", "HEAD")
			pushed = true
			return gitSyncResultMsg{pulled: pulled, pushed: pushed, pushErr: pushErr}
		}

		return gitSyncResultMsg{pulled: pulled, pushed: false}
	}
}

// syncConflictName returns "name.sync-conflict.ext" for "name.ext",
// or "name.sync-conflict" for files without an extension.
func syncConflictName(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name + ".sync-conflict"
	}
	base := strings.TrimSuffix(name, ext)
	return base + ".sync-conflict" + ext
}

func handleSyncConflict(vault string) gitSyncResultMsg {
	// Abort the rebase to restore local state
	gitCmd(vault, "rebase", "--abort")

	// Find files that differ between local and remote
	out, err := gitCmd(vault, "diff", "--name-only", "HEAD", "origin/HEAD")
	if err != nil || out == "" {
		return gitSyncResultMsg{err: fmt.Errorf("sync conflict: could not identify conflicting files")}
	}

	// Merge remote into local, preferring local version on conflicts.
	// This properly integrates remote history so we can push without --force.
	if _, err := gitCmd(vault, "merge", "origin/HEAD", "-X", "ours", "--no-edit"); err != nil {
		return gitSyncResultMsg{err: fmt.Errorf("sync conflict: merge failed")}
	}

	// Write remote versions of conflicting files as .sync-conflict copies
	files := strings.Split(out, "\n")
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// Get the remote version of the file
		remoteContent, err := gitCmd(vault, "show", "origin/HEAD:"+f)
		if err != nil {
			continue // file may have been deleted on remote
		}
		// Write as sync-conflict file
		conflictName := syncConflictName(filepath.Base(f))
		conflictPath := filepath.Join(vault, filepath.Dir(f), conflictName)
		os.WriteFile(conflictPath, []byte(remoteContent), 0o644)
	}

	// Commit conflict files and push
	gitCmd(vault, "add", "-A")
	timestamp := time.Now().Format("2006-01-02 15:04")
	gitCmd(vault, "commit", "-m", fmt.Sprintf("clipad sync: resolved conflicts %s", timestamp))
	_, pushErr := gitCmd(vault, "push", "-u", "origin", "HEAD")

	return gitSyncResultMsg{pulled: true, pushed: true, pushErr: pushErr}
}
