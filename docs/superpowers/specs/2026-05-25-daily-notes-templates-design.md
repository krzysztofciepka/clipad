# Daily Notes + Templates — Design

**Task:** 25 (Prywatne)
**Date:** 2026-05-25
**Status:** Approved

## Summary

Add a lightweight template system to clipad plus two keystroke entry points:

- **Alt+D** — open/create today's daily note at `<vault>/daily/YYYY-MM-DD.md`, created from a template when absent.
- **Alt+T** — new note from a template: pick a template, name the file, create it in the current tree folder.

> **Keybinding note:** The original task requested `Ctrl+=` and `Ctrl+Shift+=`. In a terminal TUI, Ctrl combined with a *symbol* key (`Shift+=` is `+`) is not delivered reliably without the Kitty keyboard protocol, which clipad does not enable. We use `Alt+D` / `Alt+T` instead: no Alt bindings are currently used, they are reliable across terminals, and they are mnemonic (**D**aily, **T**emplate).

## Template engine (`templates.go`)

A small `regexp`-based renderer — chosen over Go's `text/template` (its `{{.Field}}` / `{{func arg}}` syntax can't express the task's `{{date:LAYOUT}}` colon form) and over plain `strings.Replace` (can't handle the arbitrary `:format` argument).

### Variables

| Placeholder        | Expansion                                  |
|--------------------|--------------------------------------------|
| `{{date}}`         | today, `2006-01-02` (local)                |
| `{{date:LAYOUT}}`  | `now.Format(LAYOUT)`, Go reference layout  |
| `{{time}}`         | `15:04` (local)                            |
| `{{yesterday}}`    | yesterday, `2006-01-02`                    |
| `{{vault}}`        | absolute vault path                        |

- Regex: `\{\{(date|time|yesterday|vault)(?::([^{}]*))?\}\}`.
- **Unknown placeholders are left untouched** — predictable and non-destructive.
- Signature: `renderTemplate(content string, now time.Time, vault string) string`. `now` is injected (not read from `time.Now()` inside) so rendering is deterministic and unit-testable.

## Storage & seeding

- Templates live in `templatesDir()` → `$XDG_CONFIG_HOME/clipad/templates` (fallback `~/.config/clipad/templates`), mirroring `shortcutsPath()`.
- Default `daily.md` is embedded via `//go:embed defaults/daily.md`, mirroring `defaults/ai_shortcuts.toml`.
- `seedDefaultTemplate()` is idempotent: creates `templatesDir` and writes `daily.md` **only if absent** (never overwrites). Called at the start of both flows, so the daily template always exists even if the user deletes it.
- `listTemplates()` returns sorted `*.md` basenames from `templatesDir`.

### Default `daily.md`

```
# {{date:Monday, 2 January 2006}}

## Notes

## Tasks
- [ ] 

---
Yesterday: [[{{yesterday}}]]
```

## Flow: Alt+D — today's daily note

- New `case "alt+d"` in the global key switch in `model.go` → `m.openDailyNote()`.
- `openDailyNote()`:
  1. `seedDefaultTemplate()`.
  2. `path = <vault>/daily/<today:2006-01-02>.md`.
  3. If `path` exists → `openFile(path)`.
  4. Else: render `daily.md` with `renderTemplate`, `os.MkdirAll(<vault>/daily, 0o755)`, write the file, `openFile(path)`, `refreshTree()`.
- Errors surface via `m.errMsg` (existing pattern). An existing daily note is opened as-is (template is **not** re-applied).

## Flow: Alt+T — new note from template

- New `case "alt+t"` → `seedDefaultTemplate()`, load `listTemplates()`, open a **picker modal**.
  - New `inputMode` value `inputTemplatePick`; state fields `templateList []string`, `templateCursor int`.
  - Picker view in `templates_modal.go`, reusing the `shortcuts_modal` / cursor-highlight pattern.
- On select → switch to `inputMode` `inputTemplateName`: a `textinput` filename prompt rendered in the status bar, reusing the `git_sync_input` pattern.
- On enter:
  1. Target dir = current tree folder — reuse `startNewNote()`'s logic (selected node's dir, or its parent if a file is selected, default vault root).
  2. Append `.md` to the typed name if missing.
  3. If the target file exists → error via `m.errMsg`, stay in prompt.
  4. Render the chosen template, write, `openFile`, `refreshTree`.
- `esc` cancels at either stage (returns to `inputNone`).
- Handlers in `templates_input.go`, routed from `handleInputMode`.

## Help + tests

- Add to the **Global** section of `help_modal.go`:
  - `{"Alt+D", "Open today's daily note"}`
  - `{"Alt+T", "New note from template"}`
- `templates_test.go`:
  - Variable substitution: `{{date}}`, `{{date:LAYOUT}}` (custom layout), `{{time}}`, `{{yesterday}}`, `{{vault}}`, unknown-placeholder passthrough — all with a fixed `now`.
  - Seeding: creates `daily.md` when absent; never overwrites an existing file.
  - `listTemplates`: sorting and `*.md`-only filtering.
- Input/modal handler test mirroring existing `*_input_test.go` style (picker navigation + filename → create).

## Out of scope (YAGNI)

- Configurable `daily/` folder name (hardcoded per task).
- `{{yesterday:LAYOUT}}` format variant.
- strftime-style format strings (Go reference layout only).
- Eager seeding at app startup (seeding is lazy, on first feature use).

## Touched files

- New: `templates.go`, `templates_input.go`, `templates_modal.go`, `defaults/daily.md`, `templates_test.go`.
- Modified: `model.go` (key cases, `inputMode` values, state fields, `handleInputMode` routing), `help_modal.go`.
