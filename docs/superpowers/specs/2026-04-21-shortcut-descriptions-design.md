# AI Shortcut Descriptions — Design

## Problem

The Ctrl+G modal lists AI shortcuts by name only. Names like `prd`, `tldr`, `critique` don't convey what each shortcut does. Users either memorize what they wrote or open the prompt editor to find out.

## Goal

Show a short description next to each shortcut name in the selector so users can scan and pick the right one.

## Scope

- Schema: one new field on `AIShortcut`.
- Edit/create flow: one new required step between name and prompt.
- Selector rendering: inline dim description in a two-column layout.
- Defaults: seed descriptions for all 23 entries in `defaults/ai_shortcuts.toml`.

Out of scope: auto-deriving descriptions from prompts, description-based search/filter, multi-line descriptions.

## Data model

Add a `Description` field to `AIShortcut` in `shortcuts.go`:

```go
type AIShortcut struct {
    Name        string `toml:"name"`
    Description string `toml:"description"`
    Prompt      string `toml:"prompt"`
}
```

**Backward compatibility.** TOML unmarshaling treats missing keys as zero value, so a user's existing `~/.config/clipad/ai_shortcuts.toml` without `description` loads with `Description == ""`. These entries render name-only in the modal and are required to supply a description the next time they're edited.

**Defaults.** `defaults/ai_shortcuts.toml` gains a one-line description for each entry. Descriptions are short (target ≤60 chars) and written to read well after the em-dash. Since seeded-on-first-run users will get the new file, and the existing seeded-default test (`shortcuts_test.go`) already round-trips through load/save, adding the field is picked up automatically — but we still add explicit assertions that every seeded default has a non-empty description.

## Edit / create flow

Current flow (`shortcuts_input.go`): name input → prompt input → save.

New flow: name input → **description input** → prompt input → save.

Changes:

- New `inputMode` constant `inputShortcutDescription`.
- New `shortcutDescriptionInput textinput.Model` on `model`.
- New `shortcutTempDescription string` on `model` (mirrors `shortcutTempName`).
- New handler `handleShortcutDescription` that mirrors `handleShortcutName`: Enter on empty does nothing; Enter with value advances to prompt; Esc cancels; Ctrl+Q behaves as elsewhere.
- `handleShortcutName` advances to `inputShortcutDescription` instead of `inputShortcutPrompt`.
- `handleShortcutPrompt` Enter-to-save constructs `AIShortcut{Name: m.shortcutTempName, Description: m.shortcutTempDescription, Prompt: prompt}`.
- On edit (`e` in handler), prefill description input from existing value.

Validation: description is required (empty blocks the Enter advance), matching how name and prompt are handled today.

## Selector rendering

`shortcutSelectorView` in `shortcuts_modal.go` currently renders `> name` or `  name`. New format:

```
> prd    — Turn text into a PRD with TBDs for gaps
  tldr   — Add a TL;DR at the top
  ...
```

Layout rules:

1. Compute `nameCol = max(len(s.Name) for s in shortcuts) + 2`.
2. Render name padded to `nameCol`.
3. If `s.Description != ""`, append `— <description>` styled with `lipgloss.Color("240")` (same dim as `shortcutHintStyle`).
4. If the full rendered line exceeds `width - padding`, truncate the description tail with `…`. Name is never truncated.
5. Empty description → render just the padded name (no em-dash). Covers grandfathered user entries.

The cursor row uses `shortcutCursorStyle` for the name portion; the description styling is applied independently so the dim color is consistent between selected and unselected rows (the background changes via the cursor style, the foreground for the description stays dim).

## Defaults copy

Each of the 23 seeded shortcuts gets a description. Examples:

| Name | Description |
|------|-------------|
| prd | Turn text into a PRD with TBDs for gaps |
| userstory | Rewrite as user stories with acceptance criteria |
| acceptance | Write Gherkin acceptance scenarios |
| critique | Review as a draft spec and flag issues |
| todos | Extract actionable items as a checkbox list |
| prioritize | Re-rank todos into Now / Next / Later |
| breakdown | Decompose a goal into nested subtasks |
| onboard | Rewrite as an onboarding doc for new engineers |
| explain | Rewrite as a clear ground-up explainer |
| tighten | Cut filler; keep meaning; shorter |
| tldr | Add a TL;DR at the top |
| outline | Produce a nested outline of topics |
| questions | List open questions and TBDs |
| examples | Add concrete examples inline after claims |
| diagram | Insert Mermaid diagrams where they help |
| glossary | Add a glossary of domain terms at the end |
| risks | List risks, gotchas, and failure modes |
| bullets | Convert prose into a bullet list |
| steps | Convert into a numbered step list |
| table | Convert parallel structure into a table |
| headers | Insert section headers by topic |
| fmtjson | Pretty-print JSON blocks in the text |
| markdown | Clean up markdown formatting only |

Final copy tuned during implementation; the rule is one short phrase that completes the sentence "This shortcut …".

## Testing

- **`shortcuts_test.go`** — extend round-trip test with a description field. Extend the seeded-default test to assert every default has non-empty `Description`.
- **`shortcuts_modal_test.go`** (new) — render-based tests for `shortcutSelectorView`:
  - Names align to the same column (longest name + 2).
  - Description dim styling appears when set.
  - Empty description falls back to name-only (no em-dash).
  - Long descriptions truncate with `…` at narrow width.
  - Cursor row styling still applied.
- **`shortcuts_input_test.go`** (new or extended) — drive the handlers through a create and an edit to verify the new `inputShortcutDescription` step: empty Enter is a no-op, non-empty Enter advances, Esc cancels, edit prefills.
- **`shortcut_provider_test.go`** — unchanged; provider selection logic is untouched.

## Files touched

- `shortcuts.go` — add `Description` field.
- `shortcuts_input.go` — new description step, handler, prefill on edit.
- `shortcuts_modal.go` — new rendering layout.
- `model.go` — new field(s) for description input and temp value; new input-mode constant.
- `defaults/ai_shortcuts.toml` — add description to each entry.
- `shortcuts_test.go` — updated assertions.
- `shortcuts_modal_test.go` — new.
- `shortcuts_input_test.go` — new or extended.

## Non-goals

- No search/filter by description.
- No AI-generated descriptions.
- No multi-line descriptions.
- No migration tool to backfill descriptions into users' existing shortcuts — grandfathered entries work name-only until the user edits them, at which point the required field kicks in.
