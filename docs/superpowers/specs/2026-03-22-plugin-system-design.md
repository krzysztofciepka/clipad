# Clipad Plugin System Design Spec

## Overview

A built-in plugin system that allows processing notes through external services. Plugins are Go structs compiled into the binary. The first plugin integrates OpenRouter for LLM-powered note transformation (rephrase, translate, redraft). Changes are shown as a side-by-side diff with accept/reject.

## Plugin Interface

```go
type ConfigField struct {
    Key         string
    Label       string
    Placeholder string
    Secret      bool   // mask input with ***
}

type Plugin interface {
    Name() string
    Description() string
    ConfigFields() []ConfigField
    Run(content string, prompt string, config map[string]string) (string, error)
}
```

Plugins are registered explicitly in `main.go` at startup as a `[]Plugin` slice. No self-registration or discovery.

## Configuration

### Storage

Each plugin stores config in its own file: `$XDG_CONFIG_HOME/clipad/plugins/<name>.toml` (defaults to `~/.config/clipad/plugins/<name>.toml`).

Config is a flat `map[string]string` managed by shared helpers in `plugin.go`, not by the plugin itself:
- `loadPluginConfig(name string) (map[string]string, error)`
- `savePluginConfig(name string, values map[string]string) error`

### First-Use Flow

When a plugin is selected and has no config file (or is missing required fields), the app enters `inputPluginConfig` mode. It prompts for each `ConfigField` sequentially in the bottom bar. Secret fields mask input with `***`. After all fields are provided, config is saved and the flow continues to the prompt input.

## Modal Flow

### 1. Plugin Selector (`Ctrl+Space`)

- New input mode: `inputPluginSelect`
- Full-height modal overlaying the editor area (tree panel stays visible)
- Lists all registered plugins: name + description per line
- Arrow keys navigate, Enter selects, Esc cancels
- Only available when a file is open (currentFile != "" or newNoteDir != "")
- On selection: check config → if missing, enter config flow → then enter prompt flow

### 2. Config Prompt (first use)

- New input mode: `inputPluginConfig`
- Sequential prompts in the bottom bar, one per `ConfigField`
- Format: `{Label}: ___` (with placeholder text)
- Secret fields: `textinput.EchoPassword` mode
- Enter confirms the current field (empty values are rejected — field re-prompts) and moves to the next
- Esc aborts the entire config flow, returns to normal mode
- After all fields: save config to disk, proceed to prompt input

### 3. Prompt Input

- New input mode: `inputPluginPrompt`
- Text input in the bottom bar: `Prompt: ___`
- Prompt char limit: 500 characters
- Enter submits, Esc cancels and returns to normal mode
- On submit: launch plugin via a `tea.Cmd` (see Processing State), set `pluginProcessing = true`

### 4. Processing State

- `pluginProcessing bool` on the model
- While true, status bar shows "Processing..." and **all key input is suppressed** (guard at top of `Update()` before input mode dispatch — only `pluginResultMsg` is processed)
- HTTP timeout: 60 seconds via `http.Client.Timeout`
- Plugin execution uses Bubble Tea's `tea.Cmd` pattern, not a raw goroutine:

```go
func runPluginCmd(p Plugin, content, prompt string, cfg map[string]string) tea.Cmd {
    return func() tea.Msg {
        result, err := p.Run(content, prompt, cfg)
        return pluginResultMsg{result: result, err: err}
    }
}
```

The prompt handler returns this `tea.Cmd`, and Bubble Tea runs it in a goroutine automatically.

```go
type pluginResultMsg struct {
    result string
    err    error
}
```

A new case for `pluginResultMsg` must be added to the top-level `Update()` method (alongside `tea.WindowSizeMsg` and `tea.KeyMsg`).

- On error: set `pluginProcessing = false`, show error in status bar, return to normal mode
- On success: set `pluginProcessing = false`, enter diff view
- If plugin returns identical content: skip diff, show "No changes" in status bar, return to normal mode

### 5. Diff View

- New input mode: `inputPluginDiff`
- Side-by-side layout: original on the left (50%), new version on the right (50%)
- Both displayed in `viewport.Model` instances, scrollable
- Synchronized scrolling (arrow keys / j,k scroll both viewports)
- Bottom bar: `Accept changes? (y/n)`
- `y`: replace editor content with the new version, mark dirty, return to edit mode. For new unsaved notes (`newNoteDir != ""`), the `newNoteDir` and `dirty` flags are preserved — the filename is still derived from the (potentially rewritten) first line on save.
- `n`: discard result, return to editor unchanged
- On terminal resize during diff: `recalcLayout()` must also resize both diff viewports to `(editorWidth / 2, editorHeight)`

## New Input Modes

Added to the `inputMode` enum:

| Mode | Trigger | Handler |
|------|---------|---------|
| `inputPluginSelect` | `Ctrl+Space` | `handlePluginSelect` |
| `inputPluginConfig` | Auto (missing config) | `handlePluginConfig` |
| `inputPluginPrompt` | Auto (after config) | `handlePluginPrompt` |
| `inputPluginDiff` | Auto (after result) | `handlePluginDiff` |

Processing state is tracked via `pluginProcessing` bool, not an input mode.

All four new plugin input mode handlers must be added as cases in `handleInputMode()`. Each handler should pass through `Ctrl+Q`/`Ctrl+C` to the quit flow (with unsaved guard if dirty), following the pattern established by `handleFilterInput`.

## New Model Fields

```go
// Plugin system
plugins            []Plugin
pluginCursor       int
pluginActive       Plugin
pluginPrompt       textinput.Model
pluginConfigFields []ConfigField
pluginConfigIndex  int
pluginConfigValues map[string]string
pluginConfigInput  textinput.Model
pluginDiffOriginal string
pluginDiffResult   string
pluginDiffViewL    viewport.Model
pluginDiffViewR    viewport.Model
pluginProcessing   bool
```

## OpenRouter Plugin

### Config Fields

| Key | Label | Placeholder | Secret |
|-----|-------|-------------|--------|
| `api_key` | API Key | `sk-or-...` | true |
| `model` | Model | `openai/gpt-4o` | false |

### API Integration

HTTP POST to `https://openrouter.ai/api/v1/chat/completions`:

```json
{
  "model": "<configured model>",
  "messages": [
    {"role": "system", "content": "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations."},
    {"role": "user", "content": "Instruction: <user's prompt>\n\nNote:\n<full note content>"}
  ]
}
```

Headers:
- `Authorization: Bearer <api_key>`
- `Content-Type: application/json`

Response: extract `choices[0].message.content`.

No streaming. Uses Go's `net/http` and `encoding/json` — no external SDK.

### Error Handling

- HTTP non-200 responses: return error with status code and response body excerpt
- Network errors: return wrapped error
- Malformed JSON: return parse error
- Empty/missing choices: return descriptive error

All errors surface via `pluginResultMsg.err`, displayed in the status bar.

## Project Structure

| File | Responsibility |
|------|----------------|
| `plugin.go` | Plugin interface, ConfigField struct, config load/save helpers, `runPluginCmd`, `pluginResultMsg` |
| `plugin_openrouter.go` | OpenRouterPlugin struct implementing Plugin interface |
| `plugin_modal.go` | Plugin selector modal rendering and navigation |
| `plugin_input.go` | Prompt input and config prompt flow handlers (both are bottom-bar text inputs) |
| `plugin_diff.go` | Side-by-side diff view with accept/reject |
| `model.go` | Modified: new input modes, Ctrl+Space, plugin state fields, `pluginResultMsg` case in `Update()` |
| `statusbar.go` | Modified: show `^Space` in keybindings |

## Keybinding Changes

### Status Bar

Add `^Space plugins` to the keybinding hints (shown when a file is open).

### Global Keybindings

| Key | Action |
|-----|--------|
| `Ctrl+Space` | Open plugin selector (when file is open) |

`Ctrl+Space` is only handled in the global keybinding section of `Update()`, after the `inputMode != inputNone` check. It is naturally ignored during any active input mode (filter, delete confirm, unsaved guard, plugin modes). It is also ignored when `pluginProcessing == true`.

Note: some terminal emulators may intercept `Ctrl+Space`. This is a known limitation.

## Known Limitations (v1)

- No streaming — waits for full response before showing diff
- No plugin-to-plugin chaining
- No undo after accepting diff (but file isn't saved until Ctrl+S)
- Config editing requires deleting the plugin's TOML file and re-running
