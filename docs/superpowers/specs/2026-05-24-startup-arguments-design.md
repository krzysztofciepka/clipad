# Startup Arguments — Design

**Date:** 2026-05-24
**Task:** Task 58

## Goal

Extend clipad's command line so it can perform a quick action at launch instead
of always opening to the default vault view:

1. `clipad -p <path>` — open `<path>` in **preview mode** with the file tree
   **hidden**. Typing any character switches to edit mode.
2. `clipad --new` (alias `clipad -n`) — open in **new-note mode**, identical to
   clicking "+ Add note" on the file tree. The tree stays visible.
3. `clipad <path>` — open `<path>` in **edit mode** with the file tree **hidden**.

Paths may be relative or absolute and may point **anywhere on the filesystem**
(not restricted to the configured vault). Files outside the vault simply will
not appear in the tree when it is toggled visible with `Ctrl+B`.

## Path semantics

Given the resolved absolute path:

| Path state | Action |
|------------|--------|
| Existing **file** | Open it (preview if `-p`, otherwise edit). |
| Existing **directory** | Start a **new note** in that directory (same as "+ Add note" with that directory selected). |
| Non-existing path | Create an empty file (plus any missing parent directories) and open it. |
| Non-existing path ending in `/` | Create the directory (`mkdir -p`) and start a new note in it. |

Directory handling is the same for both `clipad -p <dir>` and `clipad <dir>`:
because a new note begins empty, the `-p` (preview) flag is a no-op for
directories — both forms land in new-note edit mode in that directory.

## CLI surface (`main.go`)

Stdlib `flag` is reused (it already drives `--version` / `--upgrade`). New flags:

- `-p` / `--preview` (bool) — preview the path.
- `-n` / `--new` (bool) — new-note mode.

The path is the first positional argument, `flag.Arg(0)`.

Resolution of the command forms:

- `--new` / `-n` set → new note in the **vault root**, tree **visible**. Any
  positional path and `-p` are ignored.
- `-p <path>` → preview/open `<path>`, tree **hidden**.
- `<path>` (no flags) → edit `<path>`, tree **hidden**.
- `-p` with **no** path → error to stderr, exit non-zero.
- No flags and no path → normal launch (unchanged behaviour).

Constraint (stdlib `flag` limitation, documented in README): flags must precede
the path, e.g. `clipad -p /path/to/file`. This matches every example in the
task.

## Path resolution & classification — pure function

```go
type startupKind int

const (
    startupNone startupKind = iota
    startupNewNote      // --new: new note in vault root
    startupNewNoteInDir // path is (or becomes) a directory
    startupOpenFile     // path is (or becomes) a file
)

type startupAction struct {
    kind       startupKind
    path       string // resolved absolute path (file path, or directory)
    preview    bool   // open the file in preview mode (only meaningful for startupOpenFile)
    hideTree   bool   // hide the file tree on launch
    needsCreate bool  // create an empty file (+ parents) before opening
    needsMkdir  bool  // create the directory before starting the new note
}

func resolveStartup(preview, newNote bool, pathArg, cwd, vault string) (startupAction, error)
```

`resolveStartup` is **pure** apart from `os.Stat` reads (no writes):

- Expands a leading `~` to the home directory.
- Resolves a relative `pathArg` against `cwd` to an absolute path.
- Classifies via `os.Stat` per the table above.
- Sets `preview` only for `startupOpenFile`.
- Sets `hideTree = true` for the path forms, `false` for `--new`.

`cwd` and `vault` are passed in (not read from globals) so tests need no
`chdir` and can run in parallel.

## Filesystem preparation — `main()`

```go
func prepareStartup(a startupAction) error
```

Called in `main()` after `resolveStartup`, before launching the TUI:

- `needsCreate` → `os.MkdirAll(parent)` then create an empty file.
- `needsMkdir` → `os.MkdirAll(path)`.
- Otherwise no-op.

Errors are printed to stderr and cause a non-zero exit. By the time the TUI
runs, all referenced paths exist, so the in-app apply step never has to create
anything.

## Model wiring — deferred apply

Preview viewport dimensions are only known after the first `tea.WindowSizeMsg`
(handled at `model.go:477`, which calls `recalcLayout()` to set
`editorWidth`/`editorHeight`). The startup action is therefore applied there,
not in `main()`.

- Two new model fields: `startup startupAction` and `startupDone bool`.
- `main.go` sets `m.startup = action` after `newModel(...)` — `newModel`'s
  signature is unchanged, so existing tests are untouched.
- In the `WindowSizeMsg` handler, after `recalcLayout()`:

  ```go
  if !m.startupDone && m.startup.kind != startupNone {
      m.applyStartup()
      m.startupDone = true
  }
  ```

- `applyStartup()` sets **view state only** (all paths already exist):
  - `startupOpenFile`: `m.openFile(path)`; if `preview`, build the preview
    viewport via `newPreviewViewport` (glamour-rendered Markdown, same as
    `Ctrl+P` / `togglePreview` — not raw text), set `editorMode = modePreview`,
    blur the editor, `activePanel = editorPanel` (so a keystroke in preview
    routes to `handleEditorKeys` and switches to edit); on render failure fall
    back to edit mode. Otherwise `editorMode = modeEdit`,
    `activePanel = editorPanel`, focus the editor.
  - `startupNewNote` / `startupNewNoteInDir`: set `m.newNoteDir = path`
    (vault root for `--new`), clear the editor, `editorMode = modeEdit`,
    `activePanel = editorPanel`, focus the editor.
  - Apply `m.treeHidden = action.hideTree`.

Typing-switches-to-edit in preview already works: in `handleEditorKeys`
(`model.go:962`), `modePreview` + a rune key switches to `modeEdit`, and key
routing (`model.go:853`) sends keys to `handleEditorKeys` whenever
`activePanel == editorPanel`. No new behaviour is needed for that requirement.

## Error handling

- Invalid flag combinations (`-p` with no path) → stderr message + non-zero exit
  in `main()`, before the TUI starts.
- Filesystem creation failures (`prepareStartup`) → stderr + non-zero exit.
- A misconfigured/missing vault still triggers the existing first-run setup flow;
  the startup action is applied afterwards once the model is built.

## Testing (TDD)

- `resolveStartup` unit tests: absolute path, relative path (via `cwd` param),
  `~` expansion, existing file, existing directory, missing file, trailing-slash
  missing directory, `-p` flag, `--new`, and `hideTree` per form.
- `prepareStartup` tests: creates file + missing parents, creates directory,
  idempotent on existing targets, surfaces errors.
- Model tests: set `m.startup`, feed a `WindowSizeMsg`, and assert
  `editorMode`, `activePanel`, `treeHidden`, `currentFile`, `newNoteDir`, and
  preview content for each action kind. `startupDone` guards a single apply.

## Documentation & release

- README: add a "Quick actions (CLI)" section documenting the three forms and
  the flags-before-path constraint.
- Cut a new `gh` release with a version bump that includes a **linux/amd64
  binary asset** (per the project's release policy).

## Out of scope

- Interspersed flags after the path (would require replacing stdlib `flag`).
- Selecting/highlighting an in-vault target in the (hidden) tree.
- Opening multiple files / split views.
