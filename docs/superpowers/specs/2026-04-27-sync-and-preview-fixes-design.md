# Sync conflict noise + preview-mode terminal escape fix

Date: 2026-04-27

Two independent bugs fixed under one spec because they share no code or
tests, but ship together.

## Bug 1: too many `.sync-conflict` files

### Symptom

After running git sync, several `.sync-conflict` copies appear in the vault
even when the remote only deleted files or edited files the local side did
not touch.

### Root cause

`runGitSync` in `git_sync.go` runs `git pull --rebase` against a working
tree that may still contain uncommitted local changes. A dirty working tree
makes pull-rebase abort, dropping the flow into `handleSyncConflict`.

`handleSyncConflict` then runs

    git diff --name-only HEAD origin/HEAD

and writes a `.sync-conflict` copy for **every** file in that list — which
includes files the remote modified independently and files where the
remote-only deletion just happened to differ from local. Truly-conflicting
files (both sides edited the same lines) are not distinguished from
non-conflicting ones, so the user sees a wave of `.sync-conflict` files.

### Fix

Replace the pull-rebase + post-hoc-diff approach with a commit-then-merge
flow that lets git itself report which files are actually in conflict:

1. `git fetch origin`.
2. Stage and commit local changes first
   (`git add -A`, `git commit -m "clipad backup: <ts>"`), so the working
   tree is clean before any merge.
3. `git merge --no-edit --no-ff origin/HEAD`. Git auto-handles
   fast-forward, clean three-way merges, and only conflicts when both
   sides actually changed the same path in incompatible ways.
4. If the merge reports conflicts, enumerate truly-conflicted paths via
   `git ls-files -u --full-name` and resolve each one based on which
   stages exist:
   - **Both modified (stages 2 and 3):** save the remote version
     (`git show :3:<path>`) as `<name>.sync-conflict.<ext>` next to the
     original; `git checkout --ours -- <path>`; stage both files.
   - **We modified, they deleted (only stage 2):** keep local;
     `git add <path>`; no `.sync-conflict` file.
   - **We deleted, they modified (only stage 3):** keep deletion;
     `git rm <path>`; no `.sync-conflict` file.
   Commit with message `clipad sync: resolved conflicts <ts>`.
5. `git push origin HEAD`.

If `git merge` fails with the literal `unrelated histories` message
(first sync against a remote that already has commits), retry once with
`--allow-unrelated-histories` — same fallback as the current code, just
moved to the merge call.

### What this drops

- `git diff --name-only HEAD origin/HEAD` for conflict detection.
- The `-X ours` merge that auto-resolves every conflict and hides
  real ones.
- The bare `pull --rebase` against a dirty tree (root trigger).

### Tests

Add to `git_sync_test.go`:

- `TestRunGitSync_RemoteDelete_NoConflict` — remote deletes a file local
  did not touch; the file is removed locally, no `.sync-conflict` is
  written, push succeeds.
- `TestRunGitSync_RemoteEditUnrelated_NoConflict` — remote edits file A,
  local edits file B; both apply cleanly; no `.sync-conflict` written.
- `TestRunGitSync_ModifyDelete_KeepLocal` — local modifies a file the
  remote deleted; local content kept, no `.sync-conflict`.

Keep `TestRunGitSync_Conflict` and `TestRunGitSync_ConflictWithExtension`
unchanged — both exercise real both-modified conflicts and should still
produce `.sync-conflict` copies.

## Bug 2: Ctrl+P freezes and leaks `]11;rgb:0000/0000/0000` into the editor

### Symptom

First press of Ctrl+P enters edit mode, freezes for ~1s, then inserts the
string `]11;rgb:0000/0000/0000` at the cursor. A second Ctrl+P works
correctly.

### Root cause

`preview.go:21` initialises glamour with `glamour.WithAutoStyle()`. On the
first render glamour calls termenv, which writes the OSC 11 query
`ESC ] 11 ; ? BEL` to stdout and reads the terminal's reply from stdin to
detect dark/light background.

While the main Bubble Tea program is running, stdin is owned by the
program's input loop, so the OSC 11 reply never reaches termenv. termenv
blocks until its internal timeout (~1s — the freeze) and then falls back
to a default style. Meanwhile the terminal's reply
(`ESC ] 11 ; rgb:0000/0000/0000 BEL`) is sitting in stdin and gets
delivered to Bubble Tea as a sequence of `KeyRunes`. In preview mode
those runes auto-switch to edit and are inserted into the editor
(`model.go:873-878`).

### Fix

Detect the terminal background once at startup, before Bubble Tea claims
stdin, and pass the result to glamour as a fixed style instead of using
auto-detection.

1. In `main.go`, immediately before
   `p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())`,
   call `termenv.HasDarkBackground()`. Cache the result by calling a
   small setter exported from `preview.go`, e.g.

       setDarkBackground(termenv.HasDarkBackground())

2. In `preview.go`, add a package-level `var darkBackground bool` and a
   `setDarkBackground(bool)` setter. Default value `true` (dark) covers
   tests and the rare case where `HasDarkBackground` is bypassed.
3. In `getRenderer`, replace
   `glamour.WithAutoStyle()` with
   `glamour.WithStandardStyle(styleName)` where `styleName` is `"dark"`
   if `darkBackground` else `"light"`.
4. Keep the existing renderer cache and lazy first-build on Ctrl+P;
   only the style source changes.

The OSC 11 query now runs once before the alt-screen is active, so its
reply lands on stdin while termenv is still reading and never reaches
Bubble Tea. The startup cost is the same ~1s in the worst case, but it
happens once at launch, not in the middle of a session.

### Tests

No new test required for the swap itself — it's a one-line config
change to glamour. Existing preview tests
(if any in `preview.go` or via model tests) continue to pass since
`getRenderer` still returns a usable renderer.

## Out of scope

- Exposing markdown style as a config option. Not requested; can be
  added later if the auto-detected default is wrong for some user.
- Changing how preview mode handles unrecognised escape sequences.
  Once the OSC reply no longer leaks, the existing rune-to-edit handling
  is fine.
- Rebase-vs-merge style preference for the sync history. The new flow
  produces standard merge commits; users who prefer linear history can
  rebase manually.
