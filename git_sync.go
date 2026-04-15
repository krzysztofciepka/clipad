package main

import (
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
