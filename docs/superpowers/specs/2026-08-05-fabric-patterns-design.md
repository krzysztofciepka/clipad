# Fabric Patterns in the AI Shortcut Picker

**Date:** 2026-08-05
**Status:** Approved

Two changes to clipad's AI surface: remove the blackbox.ai provider, and list the
user's installed [Fabric](https://github.com/danielmiessler/fabric) patterns in the
AI shortcut picker so a pattern can be run against the current note the same way a
native shortcut is.

## Goals

- Blackbox.ai disappears from the Ctrl+Space plugin picker and the Ctrl+G shortcut
  provider cycle, without breaking configs that already name it.
- The Ctrl+G picker gains a `── Fabric patterns ──` section listing every pattern
  found on disk.
- Running a pattern sends the pattern's prompt and the note to the same AI provider
  the native shortcuts use — not to the fabric CLI.
- The picker stays usable with ~250 extra rows.

## Non-goals

- Shelling out to the fabric CLI. Clipad reads pattern files directly; fabric's own
  provider/model config is not consulted.
- Editing, creating, or reordering fabric patterns from clipad. They are read-only.
- Vendoring patterns into the binary. If fabric is not installed, the section is
  simply absent.
- Exposing fabric patterns in the Ctrl+Space plugin picker. Shortcuts only.

## Part 1 — Removing blackbox

Delete `plugin_blackbox.go` and `plugin_blackbox_test.go`, and drop
`&BlackboxPlugin{}` from the plugin slice in `main.go`. The registered plugins
become `OpenRouterPlugin` and `OpenCodePlugin`; because both the Ctrl+Space picker
and the shortcut provider cycle iterate that slice, blackbox vanishes from both.

Three dependent constants and one migration:

| Location | Change |
| --- | --- |
| `config.go` | `defaultAIShortcutProvider = "openrouter"` |
| `shortcuts.go` | `shortcutProviderURL`'s `default:` case returns `defaultOpenRouterURL`; update the comment that names blackbox as the fallback |
| `shortcut_provider.go` | new `resolveShortcutProvider(name string, plugins []Plugin) string` |

`resolveShortcutProvider` returns `name` when a registered plugin has that name, and
`defaultAIShortcutProvider` otherwise. `newModel` passes `cfg.AIShortcutProvider`
through it, so an existing `ai_shortcut_provider = "blackbox"` resolves to openrouter
instead of failing at run time with `Unknown AI shortcut provider: blackbox`. The
config file is not rewritten; the resolved value is persisted the next time the user
cycles providers with `p`.

`~/.config/clipad/plugins/blackbox.toml` is left on disk untouched — removing a
provider should not delete the user's credentials.

Rationale for openrouter as the new default: it is already the default embeddings
provider, and `embeddings.go` falls back to the `OPENROUTER_API_KEY` environment
variable, so a working key is likely already present.

## Part 2 — Fabric pattern discovery

New file `fabric.go`.

```go
type FabricPattern struct {
	Name        string
	Description string
}
```

**Location.** `fabricPatternsDir()` returns `$FABRIC_CONFIG_HOME/patterns` when that
environment variable is set, otherwise `~/.config/fabric/patterns`.

**Listing.** `listFabricPatterns(dir string) []FabricPattern` performs one readdir.
An entry qualifies as a pattern when it is a directory containing a readable,
non-empty `system.md`. Results are sorted by name. A missing or unreadable directory
yields `nil` with no error surfaced — the section is simply absent from the picker.

**Descriptions.** Fabric ships `pattern_explanations.md` alongside the pattern
directories, with one line per pattern in the form:

```
12. **analyze_claims**: Analyse and rate truth claims with evidence, counter-arguments, fallacies, and final recommendations.
```

`parsePatternExplanations(data []byte) map[string]string` extracts name → description
from lines matching that shape. A missing or unparseable file means blank
descriptions, not an error.

**Loading.** `loadFabricPattern(dir, name string) (system, user string, err error)`
reads `system.md` and, when present and non-blank, `user.md`. This runs at *run* time
rather than list time, so opening the picker does not read ~250 files.

**Running.** `runFabricPatternStream(ctx, system, user, content, provider string, cfg map[string]string) (<-chan string, <-chan error)`
mirrors `runShortcutStream`, but follows fabric's own semantics rather than clipad's
instruction-wrapping convention:

- system prompt: the contents of `system.md`, verbatim
- user message: the note content, prefixed with `user.md` plus a blank line when
  `user.md` is non-empty

The provider URL, API key, and model come from the same `shortcutProviderURL` /
plugin-config path the native shortcuts use, so a pattern run is indistinguishable
from a shortcut run at the network layer.

## Part 3 — Picker rework

Today `m.shortcutCursor` indexes `m.shortcuts` directly. With a section header and
patterns interleaved, it becomes an index into a flat row list.

New file `shortcut_rows.go`:

```go
type rowKind int

const (
	rowShortcut rowKind = iota
	rowHeader
	rowFabric
)

type shortcutRow struct {
	kind        rowKind
	index       int // into m.shortcuts (rowShortcut) or m.fabricPatterns (rowFabric); -1 for rowHeader
	name        string
	description string
}

func buildShortcutRows(shortcuts []AIShortcut, patterns []FabricPattern, filter string) []shortcutRow
```

Row order: user shortcuts, then a `── Fabric patterns ──` header, then patterns. The
header is emitted only when at least one pattern row follows it, so an empty or
fully-filtered-out pattern set leaves the picker looking exactly as it does today.

`rowHeader` is not selectable. Cursor movement steps over it in both directions, and
a helper `nextSelectableRow(rows []shortcutRow, from, delta int) int` owns that logic
so the view and the key handler cannot disagree.

**Filtering.** When `filter` is non-empty, `buildShortcutRows` narrows each group
with `fuzzy.FindFrom` — the same `github.com/sahilm/fuzzy` package `filter.go`
already uses for the file tree — matching against pattern and shortcut names.

**Scrolling.** `m.shortcutOffset` holds the first visible row. A helper
`clampShortcutOffset(cursor, offset, visible int) int` keeps the cursor inside the
window, adjusted on every cursor move exactly as `m.filterOffset` is in the tree
filter. Visible row count is the modal height minus the footer lines (provider line
+ hint line, plus the filter line while filtering). `shortcutSelectorView` renders
only `rows[offset : offset+visible]`.

**Key handling** in `handleShortcutSelect`:

| Key | Behaviour |
| --- | --- |
| `up`/`k`, `down`/`j` | move to the next selectable row, skipping the header |
| `enter` on `rowShortcut` | unchanged: existing replace/review flow |
| `enter` on `rowFabric` | load the pattern, stream it in review mode |
| `e`, `d`, `ctrl+↑`, `ctrl+↓` on `rowFabric` | no-op; sets `m.errMsg = "Fabric patterns are read-only"` |
| `/` | enter filter mode |
| `p`, `esc`, `ctrl+q` | unchanged |

A pattern run always uses review mode: `m.inputMode = inputPluginReview` with
`m.aiRunOnSelection = false`. Most fabric patterns are analysis and extraction
(`summarize`, `extract_wisdom`, `analyze_claims`); a replace flow would overwrite the
note with commentary. Review is read-only side-by-side with `c` to copy, which is the
right shape for all 256 without per-pattern classification. If a pattern fails to
load, `m.errMsg` reports it and the picker stays open.

**Filter mode.** A new `inputShortcutFilter` input mode backed by a
`textinput.Model`, mirroring the tree's `inputFilter`: typed runes narrow the list,
arrow keys navigate, `enter` runs the selected row, `esc` clears the filter and
returns to `inputShortcutSelect`. Single-letter actions (`p`/`e`/`d`/`j`/`k`) are
unavailable while filtering because those keystrokes are text — same tradeoff the
tree filter already makes.

**Model state** added: `fabricPatterns []FabricPattern`, `shortcutOffset int`,
`shortcutFilterInput textinput.Model`. `fabricPatterns` is refreshed alongside
`m.shortcuts` when Ctrl+G opens the picker, so patterns added to the fabric directory
show up without restarting clipad.

**Call sites needing the row indirection.** `handleShortcutDeleteConfirm` and the
status-bar branch that renders `m.shortcuts[m.shortcutCursor].Name` both resolve
through a helper `m.selectedShortcutIndex() int`, which returns the index into
`m.shortcuts` or `-1` when the cursor is on a header or pattern row.

## Testing

New `fabric_test.go`, driven by a temp patterns directory:

- discovery skips loose files and directories without `system.md`
- discovery skips directories whose `system.md` is empty
- results are sorted by name
- a missing patterns directory returns `nil` without error
- `parsePatternExplanations` extracts name → description and tolerates a missing file
- `loadFabricPattern` returns `system.md` verbatim, and includes `user.md` only when
  non-blank
- `runFabricPatternStream` against an `httptest` server asserts `system.md` lands in
  the `system` message and the note in the `user` message — mirroring
  `plugin_openrouter_test.go`

New `shortcut_rows_test.go`:

- header suppressed when the pattern set is empty
- header suppressed when the filter excludes every pattern
- filter narrows shortcuts and patterns independently
- `nextSelectableRow` skips the header in both directions and stops at the ends
- `clampShortcutOffset` keeps the cursor visible scrolling down and up

Retargeted existing tests: `shortcut_provider_test.go`, `config_test.go`,
`plugin_test.go`, and `shortcuts_modal_test.go` reference blackbox today and move to
openrouter/opencode. `shortcut_provider_test.go` gains coverage for
`resolveShortcutProvider` mapping an unknown name to the default.

## Documentation

- `README.md`: remove the `### Blackbox` provider section and blackbox mentions in
  the feature list, the provider-cycle paragraph, and the agent section. Add a Fabric
  patterns subsection covering the source directory, `FABRIC_CONFIG_HOME`, review-only
  behaviour, and the `/` filter.
- `help_modal.go`: add `/` (filter) to the shortcut-picker key list and note that the
  picker lists fabric patterns.

Historical documents under `docs/superpowers/plans/` mention blackbox and are left
alone as a record of past work.
