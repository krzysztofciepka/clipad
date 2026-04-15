package main

import (
	"fmt"
	"os/exec"
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

		// Check if remote has commits we don't have
		localHead, _ := gitCmd(vault, "rev-parse", "HEAD")
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
					// Conflict — handle in Task 4
					return handleSyncConflict(vault)
				}
			}
			pulled = true
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

func handleSyncConflict(vault string) gitSyncResultMsg {
	return gitSyncResultMsg{err: fmt.Errorf("sync conflict: not yet implemented")}
}
