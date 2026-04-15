# Git Sync for Vault Files — Design Spec

## Overview

Bi-directional git synchronization for the clipad vault directory. Clipad periodically pulls remote changes and pushes local changes to a configured git remote, keeping vault contents in sync across machines.

## Architecture

A new `git_sync.go` file follows the existing async timer pattern (`autosave.go`). A 30-minute check timer fires `gitSyncCheckMsg`. The handler compares `time.Since(lastSync)` against 24 hours. If due, it runs the full sync cycle as a `tea.Cmd` goroutine, returning `gitSyncResultMsg`. On startup, the same check runs immediately so the user sees remote changes right away.

Git operations use `os/exec` to shell out to the `git` CLI, with `Dir` set to the vault path. No external Go git library is needed.

### New Files

- `git_sync.go` — timer commands, message types, git operations, conflict resolution
- `git_sync_input.go` — TUI input handler and modal rendering for remote URL prompt

### Modified Files

- `config.go` — add `GitRemote` and `LastSync` fields to `Config` struct
- `model.go` — add sync state fields, `inputGitRemote` input mode, wire into `Init()`, `Update()`, and Ctrl+Q handlers
- `statusbar.go` — render sync flash and warning messages

## Config Changes

```go
type Config struct {
    Vault    string     `toml:"vault"`
    GitRemote string   `toml:"git_remote,omitempty"`
    LastSync *time.Time `toml:"last_sync,omitempty"`
}
```

`GitRemote` stores the remote URL (e.g., `git@github.com:krzysztofciepka/clipad.git`). `LastSync` persists the timestamp of the last successful sync so it survives app restarts.

## Model State

New fields on the `model` struct:

```go
// Git sync
gitSyncRunning  bool           // true while git ops goroutine is executing
gitSyncFlash    string         // flash message text ("Synced", "Backed up", etc.)
gitSyncError    string         // persistent warning if sync failed
gitSyncQuitting bool           // true if Ctrl+Q pressed during sync
gitRemoteInput  textinput.Model
```

New `inputMode` value:

```go
inputGitRemote  // inline prompt for remote URL
```

## Message Types

```go
type gitSyncCheckMsg struct{}        // 30-min timer tick
type gitSyncResultMsg struct {       // result from git ops goroutine
    pulled  bool   // true if pull brought new commits
    pushed  bool   // true if local changes were committed and pushed
    pushErr error  // non-nil if push failed (commit preserved locally)
    err     error  // non-nil if the whole sync failed
}
type gitSyncFadeMsg struct{}         // 2s fade for status flash
```

## Timer & Scheduling

- `gitSyncCheck()` returns a `tea.Cmd` that fires `gitSyncCheckMsg` after 30 minutes.
- On startup, `Init()` includes `gitSyncCheck()` with a 0-second delay (immediate check).
- The `gitSyncCheckMsg` handler loads config, reads `LastSync`. If nil or >= 24h ago, it triggers the sync. Otherwise, it reschedules the 30-min tick.
- After a successful sync, `LastSync` is updated in config and saved.

## Sync Operations

The sync runs as a single `tea.Cmd` goroutine. Pull always runs first, then push.

### Pull

1. Check if vault is a git repo (`git rev-parse --git-dir`).
2. If not a repo and `GitRemote` is set: `git init` + `git remote add origin <url>`.
3. If not a repo and `GitRemote` is empty: return early (triggers remote URL prompt).
4. `git fetch origin`.
5. `git pull --rebase origin HEAD`. If this fails with "fatal: refusing to merge unrelated histories" (detected by exit code + stderr), retry with `--allow-unrelated-histories`. This handles the case where the remote already has commits from another machine.
6. Pulled changes are picked up by the existing FS watcher, which refreshes the file tree.

### Push

1. `git add -A` (all files in vault).
2. `git diff --cached --quiet` — if exit code 0, no changes to commit; skip push.
3. `git commit -m "clipad backup: 2026-04-15 14:30"` (timestamp of the commit).
4. `git push -u origin HEAD`.

### Result Handling

- Push failure: `commitOk` is true, commit is preserved locally. Push retries on next sync cycle.
- No changes: still counts as a successful sync — `LastSync` is updated.
- First push: `-u origin HEAD` sets upstream tracking.

## Conflict Resolution

When `git pull --rebase` fails due to conflicts:

1. `git rebase --abort` — restore local working state.
2. Identify conflicting files: `git diff --name-only HEAD origin/HEAD`.
3. For each conflicting file, extract the remote version: `git show origin/HEAD:<filepath>`.
4. Save the remote version alongside the local file as `<name>.sync-conflict.<ext>` (e.g., `Notes.sync-conflict.md`).
5. `git add -A` + `git commit -m "clipad sync: resolved conflicts"` + `git push`.
6. Report conflict in status bar.

Edge cases:
- Multiple conflicts: each file gets its own `.sync-conflict` copy.
- Existing `.sync-conflict` file from a previous unresolved conflict: overwrite it.
- Binary files: same treatment.
- Deleted locally, changed remotely: save remote version as `.sync-conflict` file.

The `.sync-conflict` files appear in the file tree via the FS watcher, making them easy to find and resolve.

## Remote URL Input

When sync is triggered and no `GitRemote` is configured:

1. Enter `inputGitRemote` mode.
2. Show a centered modal (matching plugin config input style) with prompt: "Git remote URL for vault sync:".
3. User types URL (e.g., `git@github.com:user/vault.git`) and presses Enter.
4. Save `GitRemote` to config.
5. Sync proceeds on the next check cycle.
6. Esc cancels — sync is skipped this cycle, prompt reappears on the next cycle.

The input handler in `git_sync_input.go` follows the `handlePluginConfig` pattern:
- Enter: validate non-empty, save to config, return to `inputNone`.
- Esc: cancel, return to `inputNone`.
- Other keys: forward to text input.

No URL validation — the value is passed directly to `git remote add origin <url>`.

## Graceful Shutdown

When the user presses Ctrl+Q while `gitSyncRunning` is true:

1. Set `gitSyncQuitting = true`.
2. Show "Waiting for sync to finish..." in status bar.
3. Do not issue `tea.Quit`.
4. When `gitSyncResultMsg` arrives and `gitSyncQuitting` is true, issue `tea.Quit`.

This prevents exiting mid-commit or mid-push, which could leave the git repo in a broken state.

## Status Bar Messages

| State | Message | Duration |
|---|---|---|
| Sync in progress | "Syncing..." | While running |
| Pull brought changes | "Synced from remote" | 2s flash |
| Push succeeded | "Backed up" | 2s flash |
| Pull + push | "Synced" | 2s flash |
| Push failed | "Sync: push failed" | Persistent until next success |
| Conflict resolved | "Sync conflict — check .sync-conflict files" | Persistent until next success |
| Waiting to quit | "Waiting for sync to finish..." | Until sync completes |

## Assumptions

- Git is installed and available in `PATH`.
- SSH keys or credential helpers are configured by the user for the remote.
- The vault directory exists and is accessible.
- The remote repository exists (clipad does not create GitHub repos).
