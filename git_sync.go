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

		// Fetch remote
		gitCmd(vault, "fetch", "origin")

		// 1. Stage and commit local changes BEFORE merging, so the working
		//    tree is clean and merge can compute a real three-way result.
		if _, err := gitCmd(vault, "add", "-A"); err != nil {
			return gitSyncResultMsg{err: fmt.Errorf("git add: %w", err)}
		}
		if _, err := gitCmd(vault, "diff", "--cached", "--quiet"); err != nil {
			timestamp := time.Now().Format("2006-01-02 15:04")
			msg := fmt.Sprintf("clipad backup: %s", timestamp)
			if _, err := gitCmd(vault, "commit", "-m", msg); err != nil {
				return gitSyncResultMsg{err: fmt.Errorf("git commit: %w", err)}
			}
		}

		// 2. If there's no local HEAD yet (brand new repo with no commits),
		//    push directly. This handles the very-first-sync case.
		localHead, localErr := gitCmd(vault, "rev-parse", "HEAD")
		if localErr != nil {
			_, pushErr := gitCmd(vault, "push", "-u", "origin", "HEAD")
			return gitSyncResultMsg{pushed: true, pushErr: pushErr}
		}

		// 3. If remote has commits we don't have, merge them in.
		remoteHead, _ := gitCmd(vault, "rev-parse", "origin/HEAD")
		if remoteHead != "" && localHead != remoteHead {
			out, err := gitCmd(vault, "merge", "--no-edit", "--no-ff", "origin/HEAD")
			if err != nil {
				if strings.Contains(out, "unrelated histories") {
					_, err = gitCmd(vault, "merge", "--no-edit", "--no-ff",
						"--allow-unrelated-histories", "origin/HEAD")
				}
			}
			if err != nil {
				if resErr := resolveMergeConflicts(vault); resErr != nil {
					return gitSyncResultMsg{err: resErr}
				}
			}
			afterMerge, _ := gitCmd(vault, "rev-parse", "HEAD")
			if afterMerge != localHead {
				pulled = true
			}
		}

		// 4. Push if remote has no HEAD yet (first sync into an empty
		//    remote) or local is ahead.
		needPush := false
		if remoteHead == "" {
			needPush = true
		} else {
			ahead, _ := gitCmd(vault, "rev-list", "--count", "origin/HEAD..HEAD")
			if ahead != "" && ahead != "0" {
				needPush = true
			}
		}
		if needPush {
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

// resolveMergeConflicts is called after `git merge` left the index in a
// conflicted state. It enumerates conflicted paths via `git ls-files -u`
// and resolves each one based on which stages are present:
//   - stages 2 AND 3 -> both modified: write theirs as .sync-conflict
//     sibling, keep ours (`checkout --ours`).
//   - stage 2 only   -> modified by us, deleted by them: keep our edit.
//   - stage 3 only   -> deleted by us, modified by them: keep deletion.
//
// It then commits the merge so the caller can push.
func resolveMergeConflicts(vault string) error {
	out, _ := gitCmd(vault, "ls-files", "-u", "--full-name")
	if out == "" {
		gitCmd(vault, "merge", "--abort")
		return fmt.Errorf("sync conflict: merge failed with no unmerged paths")
	}

	// Each line: "<mode> <sha> <stage>\t<path>"
	stages := map[string]map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		head := line[:tab]
		path := line[tab+1:]
		fields := strings.Fields(head)
		if len(fields) < 3 {
			continue
		}
		stage := 0
		fmt.Sscanf(fields[2], "%d", &stage)
		if _, ok := stages[path]; !ok {
			stages[path] = map[int]bool{}
		}
		stages[path][stage] = true
	}

	for path, st := range stages {
		switch {
		case st[2] && st[3]:
			theirs, err := gitCmd(vault, "show", ":3:"+path)
			if err == nil {
				conflictName := syncConflictName(filepath.Base(path))
				conflictRel := filepath.Join(filepath.Dir(path), conflictName)
				conflictPath := filepath.Join(vault, conflictRel)
				os.WriteFile(conflictPath, []byte(theirs), 0o644)
				gitCmd(vault, "add", conflictRel)
			}
			gitCmd(vault, "checkout", "--ours", "--", path)
			gitCmd(vault, "add", path)
		case st[2] && !st[3]:
			gitCmd(vault, "add", path)
		case !st[2] && st[3]:
			gitCmd(vault, "rm", "-f", path)
		}
	}

	timestamp := time.Now().Format("2006-01-02 15:04")
	if _, err := gitCmd(vault, "commit", "-m",
		fmt.Sprintf("clipad sync: resolved conflicts %s", timestamp)); err != nil {
		return fmt.Errorf("sync conflict: commit failed: %w", err)
	}
	return nil
}

func (m model) triggerManualGitSync() (tea.Model, tea.Cmd) {
	if m.gitSyncRunning {
		return m, nil
	}
	cfg, err := loadConfig()
	if err != nil {
		m.errMsg = "Git sync: " + err.Error()
		return m, nil
	}
	if cfg.GitRemote == "" {
		m.inputMode = inputGitRemote
		m.gitRemoteInput.SetValue("")
		cmd := m.gitRemoteInput.Focus()
		return m, cmd
	}
	m.gitSyncRunning = true
	m.gitSyncError = ""
	return m, runGitSync(m.vault, cfg.GitRemote)
}
