# Read-Only Preview Mode

## Problem

When navigating the file tree, files are previewed in a read-only viewport. However, users cannot scroll the preview without entering edit mode. The only way to interact with the content is pressing Right/Enter, which activates full editing. Users want to read through file content without risking accidental edits.

Additionally, there is a rendering bug where scrolling the preview causes the tree panel to shift its visible offset, despite no tree state changes.

## Design

### State Model

No new types or enums. Three focus states are expressed by existing fields:

| State | `activePanel` | `editorMode` | Behavior |
|-------|--------------|-------------|----------|
| Tree browsing | `treePanel` | `modePreview` | j/k navigate tree, preview updates |
| Preview reading | `editorPanel` | `modePreview` | up/down/pgup/pgdn scroll viewport |
| Editing | `editorPanel` | `modeEdit` | Full text editing |

### Focus Transitions

```
Tree ──Tab──> Preview ──Enter/typing──> Edit
  ^              |                        |
  └────Esc───────┘                        |
  ^                                       |
  └──────────────Esc──────────────────────┘
```

- **Tab** from tree: `activePanel = editorPanel`, does NOT call `editor.Focus()`
- **Enter** from preview: `editorMode = modeEdit`, calls `editor.Focus()`
- **Printable key** from preview: same as Enter, then forwards the key to the editor
- **Right** from preview: `editorMode = modeEdit`, calls `editor.Focus()`
- **Esc** from preview: `activePanel = treePanel`
- **Esc** from edit: existing behavior (unsaved guard if dirty, then tree + modePreview)
- **Tab** from preview/edit: `activePanel = treePanel`, calls `editor.Blur()`

### Key Handling in Preview Mode

`handleEditorKeys` when `modePreview`:

| Key | Action |
|-----|--------|
| `esc` | `activePanel = treePanel` |
| `enter` | `editorMode = modeEdit`, `editor.Focus()` |
| `right` | `editorMode = modeEdit`, `editor.Focus()` |
| Printable (KeyRunes) | Switch to edit, forward key to editor |
| Everything else | `m.preview.Update(msg)` (viewport scrolls) |

Global hotkeys (Ctrl+P, Ctrl+S, Ctrl+Q, etc.) are handled before `handleEditorKeys` is reached.

### Visual Indicator

When the preview is focused (`activePanel == editorPanel`, `editorMode == modePreview`), render a colored left border on the preview panel using a `previewFocusedStyle` variant. The border color matches the tree cursor highlight (color "117"). The viewport width is reduced by 1 to account for the border character.

### Tree Scroll Bug

When scrolling the preview, the tree panel's visible offset shifts despite no tree state changes. The tree uses `MaxHeight` (caps but may not pad) while the preview uses `Height` (forces exact height). If their rendered line counts differ, `JoinHorizontal` may re-align them. Fix: ensure both panels produce consistent rendered heights.

## Files Changed

- **model.go**: Tab handler, `handleEditorKeys` preview block, `View()` preview style selection
- **editor.go**: `previewFocusedStyle` definition

## Scope

~30 lines of meaningful changes plus the tree scroll bug fix. No new files, types, or enums.
