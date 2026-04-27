# Sync conflict noise + preview OSC-leak fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop generating spurious `.sync-conflict` files for non-conflicting remote changes, and stop the OSC 11 background-color query in `glamour.WithAutoStyle()` from freezing Ctrl+P and leaking `]11;rgb:0000/0000/0000` into the editor.

**Architecture:** Two independent fixes. Bug 1 rewrites `runGitSync`/`handleSyncConflict` in `git_sync.go` to commit-then-merge (instead of pull-rebase against a dirty tree), and to enumerate truly-conflicted files via `git ls-files -u` instead of every diffing path. Bug 2 detects the terminal background once at startup in `main.go` (before `tea.Program` claims stdin), caches the result, and switches `getRenderer` in `preview.go` to use `glamour.WithStandardStyle("dark"|"light")`.

**Tech Stack:** Go 1.x, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/glamour`, `github.com/muesli/termenv`, plain `git` invoked via `os/exec`.

---

## File Map

- **Modify:** `git_sync.go` — replace the body of `runGitSync` and rewrite `handleSyncConflict` to operate on stage-aware unmerged paths.
- **Modify:** `git_sync_test.go` — add three new tests; existing tests stay.
- **Modify:** `preview.go` — add `darkBackground` package var + `setDarkBackground` setter; swap `WithAutoStyle` for `WithStandardStyle` in `getRenderer`.
- **Modify:** `main.go` — call `termenv.HasDarkBackground()` and `setDarkBackground(...)` before `tea.NewProgram(m, ...)`.

The two bugs are completely independent. Sync work (Tasks 1–6) and preview work (Tasks 7–9) can be done in either order.

---

## Bug 1: Sync conflict overhaul

### Task 1: Add red test — remote-only delete must NOT create a sync-conflict

**Files:**
- Modify: `git_sync_test.go` (append after `TestRunGitSync_RemoteChanges`)

- [ ] **Step 1: Append the failing test**

```go
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
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test ./... -run TestRunGitSync_RemoteDelete_NoConflict -v`
Expected: FAIL — current implementation either errors or creates a `.sync-conflict.md` for `doomed.md`.

- [ ] **Step 3: Commit the red test**

```bash
git add git_sync_test.go
git commit -m "test(sync): remote-only delete must not create sync-conflict file"
```

---

### Task 2: Add red test — unrelated remote edit must NOT create a sync-conflict

**Files:**
- Modify: `git_sync_test.go` (append after Task 1's test)

- [ ] **Step 1: Append the failing test**

```go
func TestRunGitSync_RemoteEditUnrelated_NoConflict(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)
	other := initLocalWithRemote(t, remote)

	// Local edits a file that "other" never touches.
	os.WriteFile(filepath.Join(local, "local-only.md"), []byte("local"), 0o644)

	// Other edits a different file and pushes.
	os.WriteFile(filepath.Join(other, "remote-only.md"), []byte("remote"), 0o644)
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "remote unrelated edit").Run()
	exec.Command("git", "-C", other, "push").Run()

	// Local syncs.
	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if !msg.pushed {
		t.Error("pushed = false, want true")
	}
	if _, err := os.Stat(filepath.Join(local, "remote-only.md")); err != nil {
		t.Errorf("remote-only.md missing locally after sync: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(local, "*sync-conflict*"))
	if len(matches) != 0 {
		t.Errorf("unexpected sync-conflict files: %v", matches)
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test ./... -run TestRunGitSync_RemoteEditUnrelated_NoConflict -v`
Expected: FAIL — current implementation either errors (dirty tree pull-rebase) or writes a sync-conflict for `remote-only.md`.

- [ ] **Step 3: Commit**

```bash
git add git_sync_test.go
git commit -m "test(sync): unrelated remote edit must not create sync-conflict file"
```

---

### Task 3: Add red test — modify/delete keeps local without a sync-conflict

**Files:**
- Modify: `git_sync_test.go` (append after Task 2's test)

- [ ] **Step 1: Append the failing test**

```go
func TestRunGitSync_ModifyDelete_KeepLocal(t *testing.T) {
	remote := initBareRemote(t)
	local := initLocalWithRemote(t, remote)
	other := initLocalWithRemote(t, remote)

	// Seed a file via "other".
	os.WriteFile(filepath.Join(other, "shared.md"), []byte("seed"), 0o644)
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "seed shared").Run()
	exec.Command("git", "-C", other, "push").Run()

	// Local pulls the seed.
	exec.Command("git", "-C", local, "pull", "--rebase", "origin", "HEAD").Run()

	// Local modifies shared.md; "other" deletes it and pushes.
	os.WriteFile(filepath.Join(local, "shared.md"), []byte("local edit"), 0o644)
	os.Remove(filepath.Join(other, "shared.md"))
	exec.Command("git", "-C", other, "add", "-A").Run()
	exec.Command("git", "-C", other, "commit", "-m", "delete shared").Run()
	exec.Command("git", "-C", other, "push").Run()

	cmd := runGitSync(local, remote)
	msg := cmd().(gitSyncResultMsg)
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	// Local content kept.
	data, err := os.ReadFile(filepath.Join(local, "shared.md"))
	if err != nil {
		t.Fatalf("shared.md missing locally: %v", err)
	}
	if string(data) != "local edit" {
		t.Errorf("shared.md = %q, want %q", string(data), "local edit")
	}
	// No sync-conflict file.
	matches, _ := filepath.Glob(filepath.Join(local, "*sync-conflict*"))
	if len(matches) != 0 {
		t.Errorf("unexpected sync-conflict files: %v", matches)
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test ./... -run TestRunGitSync_ModifyDelete_KeepLocal -v`
Expected: FAIL — current implementation either errors or produces a sync-conflict copy for `shared.md`.

- [ ] **Step 3: Commit**

```bash
git add git_sync_test.go
git commit -m "test(sync): modify/delete keeps local edit without conflict file"
```

---

### Task 4: Rewrite `runGitSync` flow — commit, then merge, then conflict-resolve, then push

**Files:**
- Modify: `git_sync.go:58-187` (replace `runGitSync` body and `handleSyncConflict`)

- [ ] **Step 1: Replace the `runGitSync` function**

In `git_sync.go`, replace the existing `runGitSync` function (lines 58–133) with:

```go
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
			// There are staged changes — commit them.
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
					out, err = gitCmd(vault, "merge", "--no-edit", "--no-ff",
						"--allow-unrelated-histories", "origin/HEAD")
				}
			}
			if err != nil {
				// Real conflict — resolve per-file based on stages.
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
```

- [ ] **Step 2: Replace `handleSyncConflict` with `resolveMergeConflicts`**

In `git_sync.go`, replace the existing `handleSyncConflict` function (lines 146–187) with:

```go
// resolveMergeConflicts is called after `git merge` left the index in a
// conflicted state. It enumerates conflicted paths via `git ls-files -u`
// and resolves each one based on which stages are present:
//   - stages 2 AND 3 -> both modified: write theirs as .sync-conflict
//                       sibling, keep ours (`checkout --ours`).
//   - stage 2 only   -> modified by us, deleted by them: keep our edit.
//   - stage 3 only   -> deleted by us, modified by them: keep deletion.
// It then commits the merge so the caller can push.
func resolveMergeConflicts(vault string) error {
	out, _ := gitCmd(vault, "ls-files", "-u", "--full-name")
	if out == "" {
		// No unmerged paths — merge produced a different failure we can't
		// auto-resolve. Abort and surface an error.
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
			// Both modified — save theirs as sync-conflict sibling.
			theirs, err := gitCmd(vault, "show", ":3:"+path)
			if err == nil {
				conflictName := syncConflictName(filepath.Base(path))
				conflictPath := filepath.Join(vault, filepath.Dir(path), conflictName)
				os.WriteFile(conflictPath, []byte(theirs), 0o644)
				gitCmd(vault, "add", filepath.Join(filepath.Dir(path), conflictName))
			}
			gitCmd(vault, "checkout", "--ours", "--", path)
			gitCmd(vault, "add", path)
		case st[2] && !st[3]:
			// Modified by us, deleted by them — keep ours.
			gitCmd(vault, "add", path)
		case !st[2] && st[3]:
			// Deleted by us, modified by them — keep deletion.
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
```

- [ ] **Step 3: Verify the package builds**

Run: `go build ./...`
Expected: clean build, no errors.

- [ ] **Step 4: Run all sync tests**

Run: `go test ./... -run TestRunGitSync -v`
Expected: PASS for all five tests including the three new ones from Tasks 1–3 and the two existing ones (`TestRunGitSync_Conflict`, `TestRunGitSync_ConflictWithExtension`).

If `TestRunGitSync_Conflict` fails because the new merge produces a merge commit message conflict on `.gitkeep`, debug: with this flow, both sides committing `.gitkeep` differently is a real both-modified conflict that should still produce a `.sync-conflict.gitkeep` and keep local content — so the existing assertions should pass.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS (no regressions in other packages).

- [ ] **Step 6: Commit**

```bash
git add git_sync.go
git commit -m "fix(sync): commit-then-merge flow + per-file conflict resolution"
```

---

### Task 5: Manual-trigger test smoke check

**Files:**
- Test only — verify `git_sync_test.go` tests in `TestTriggerManualGitSync_*` still pass.

- [ ] **Step 1: Run the manual-trigger tests**

Run: `go test ./... -run TestTriggerManualGitSync -v`
Expected: PASS for all four `TestTriggerManualGitSync_*` tests. They don't exercise the rewritten merge flow, but they share helpers, so this is a quick safety net.

- [ ] **Step 2: If anything fails, fix in-place and re-run.** No commit needed unless a fix is required.

---

### Task 6: Sync-flow code review pass

**Files:** none (review only)

- [ ] **Step 1: Re-read `git_sync.go` end-to-end.** Check that:
  - `gitCmd` results are not silently discarded where errors matter (e.g., the `gitCmd(vault, "fetch", "origin")` call deliberately ignores errors; that's intentional — the user can be offline). The new code follows the same pattern.
  - `syncConflictName` is still used so the naming stays consistent with `TestSyncConflictName` expectations.
  - The `triggerManualGitSync` model method (line ~189 in the original file) is unchanged.

- [ ] **Step 2: If you spot any issue, fix it and add a test that proves the fix.**

No commit unless a fix is needed.

---

## Bug 2: Preview background detection at startup

### Task 7: Add `darkBackground` package var and setter in `preview.go`; switch glamour style

**Files:**
- Modify: `preview.go` (entire file)

- [ ] **Step 1: Replace the contents of `preview.go`**

```go
package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var previewStyle = lipgloss.NewStyle().Padding(0, 1)

// darkBackground records whether the user's terminal has a dark
// background. It is set once at startup by main.go via setDarkBackground
// (before tea.Program claims stdin), so glamour can pick a fixed style
// without doing its own OSC 11 query mid-session.
var darkBackground = true

func setDarkBackground(dark bool) {
	darkBackground = dark
	// Invalidate the cached renderer so the next render uses the new style.
	cachedRenderer = nil
	cachedRendererWidth = 0
}

var (
	cachedRenderer      *glamour.TermRenderer
	cachedRendererWidth int
)

func getRenderer(width int) (*glamour.TermRenderer, error) {
	if cachedRenderer != nil && cachedRendererWidth == width {
		return cachedRenderer, nil
	}
	style := "dark"
	if !darkBackground {
		style = "light"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width-4),
	)
	if err != nil {
		return nil, err
	}
	cachedRenderer = r
	cachedRendererWidth = width
	return r, nil
}

func renderMarkdown(content string, width int) (string, error) {
	r, err := getRenderer(width)
	if err != nil {
		return "", err
	}
	return r.Render(content)
}

func newPreviewViewport(content string, width, height int) (viewport.Model, error) {
	rendered, err := renderMarkdown(content, width)
	if err != nil {
		return viewport.Model{}, err
	}
	vp := viewport.New(width-2, height)
	vp.SetContent(rendered)
	return vp, nil
}
```

- [ ] **Step 2: Verify the package builds**

Run: `go build ./...`
Expected: clean build, no errors.

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: PASS — no test currently exercises `getRenderer`'s style choice, but build-and-pass is the gate.

- [ ] **Step 4: Commit**

```bash
git add preview.go
git commit -m "fix(preview): use fixed glamour style based on cached background"
```

---

### Task 8: Detect terminal background once in `main.go` before tea.Program runs

**Files:**
- Modify: `main.go:170-179` (just before `p := tea.NewProgram(m, ...)`)

- [ ] **Step 1: Add the termenv import**

Locate the `import` block at the top of `main.go` (lines 3–12). Add the import:

```go
import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)
```

If `github.com/muesli/termenv` is already pulled in transitively (it is — see `go.sum`), this should work without `go get`. If not, run `go mod tidy`.

- [ ] **Step 2: Call the detection right before the main `tea.NewProgram`**

In `main.go`, find the lines:

```go
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
```

Insert immediately above them:

```go
	// Detect terminal background once, before tea.Program claims stdin.
	// Doing this inside glamour.WithAutoStyle() while the alt-screen is
	// active causes the OSC 11 reply to be delivered to Bubble Tea's
	// input loop as keyboard runes (visible as "]11;rgb:0000/0000/0000")
	// and freezes the first preview render until termenv times out.
	setDarkBackground(termenv.HasDarkBackground())

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
```

- [ ] **Step 3: Run `go mod tidy` to make sure termenv is a direct dep**

Run: `go mod tidy`
Expected: clean run; `go.mod` may switch `termenv` from `// indirect` to a direct dep (this is fine — commit the change in Step 5).

- [ ] **Step 4: Verify build + tests**

Run: `go build ./... && go test ./...`
Expected: build succeeds, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add main.go go.mod go.sum
git commit -m "fix(preview): detect terminal background at startup, before tea.Program"
```

---

### Task 9: Manual smoke test for Ctrl+P

**Files:** none (manual verification)

- [ ] **Step 1: Build and run clipad in the user's normal terminal.**

```bash
go build -o clipad . && ./clipad
```

- [ ] **Step 2: Open any markdown note, press Ctrl+P.**

Expected:
- No freeze.
- Markdown renders immediately (or after a barely-perceptible cache-miss on first render).
- No `]11;rgb:0000/0000/0000` text appears anywhere in the editor or preview.
- Press Ctrl+P again — it toggles back to edit mode cleanly.

- [ ] **Step 3: If anything goes wrong, capture details and fix.** No commit unless a fix is required.

---

## Final wrap-up

- [ ] **Step 1: Run the full test suite one more time**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 2: Glance at `git log --oneline` to confirm commits land in the order:**

  1. `test(sync): remote-only delete...`
  2. `test(sync): unrelated remote edit...`
  3. `test(sync): modify/delete keeps local...`
  4. `fix(sync): commit-then-merge flow...`
  5. `fix(preview): use fixed glamour style...`
  6. `fix(preview): detect terminal background at startup...`

  (Sync and preview commit groups can also be done in the opposite order.)

- [ ] **Step 3: Done.** No PR / merge actions in this plan — leave that to the user.
