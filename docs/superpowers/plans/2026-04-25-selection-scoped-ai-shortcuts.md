# Selection-Scoped AI Shortcuts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the plugin custom-prompt path (`Ctrl+Space` → pick plugin → enter prompt) selection-aware, matching the AI shortcut path's behavior; rename the related state field; consolidate the duplicated "selected text vs. whole content" logic into a helper; fix the README; cover both paths with tests.

**Architecture:** Additive refactor. New helper `(*model).aiInputContent()` becomes the single read site for "what content do we send to the LLM and was it scoped to a selection?" Three call sites use it. The state field `shortcutOnSelection` is renamed `aiRunOnSelection` to reflect that it now spans plugin and shortcut runs. The diff/accept handler already does the right thing once the field reflects its broader meaning.

**Tech Stack:** Go, Bubble Tea (tea.Msg-driven model), Lipgloss, standard `testing` package.

**Spec:** `docs/superpowers/specs/2026-04-25-selection-scoped-ai-shortcuts-design.md`

---

## File map

- **Modify** `model.go` — declare helper `aiInputContent()`; rename field `shortcutOnSelection` → `aiRunOnSelection`.
- **Modify** `plugin_input.go` — rename field; refactor `handlePluginConfig` shortcut-pending branch to use helper; **fix `handlePluginPrompt`** to use helper (the actual behavior change).
- **Modify** `plugin_diff.go` — rename field at three sites.
- **Modify** `shortcuts_input.go` — rename field; refactor `handleShortcutSelect` to use helper.
- **Modify** `README.md` — add `Ctrl+G` row, fix Ctrl+Space mention, add selection-scoped paragraph.
- **Create** `plugin_selection_test.go` — model-level tests with a fake plugin.

---

## Task 1: Add `aiInputContent()` helper (TDD)

**Files:**
- Modify: `model.go` (add method near other model receivers)
- Create test fixture in: `plugin_selection_test.go`

- [ ] **Step 1: Create `plugin_selection_test.go` with the helper test**

Create the file `plugin_selection_test.go` with this exact content:

```go
package main

import (
	"testing"
)

func TestAIInputContent_NoSelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	content, onSelection := m.aiInputContent()
	if content != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
	if onSelection {
		t.Error("onSelection = true, want false")
	}
}

func TestAIInputContent_WithSelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	content, onSelection := m.aiInputContent()
	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	if !onSelection {
		t.Error("onSelection = false, want true")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestAIInputContent`

Expected: build fails with `m.aiInputContent undefined`. That's the failing test signal.

- [ ] **Step 3: Add `aiInputContent` to `model.go`**

Open `model.go`. Find the existing `func (m model) isDirty() bool` method (around line 222). Add this new method directly after it:

```go
// aiInputContent returns the content to feed to an AI run plus a flag the
// diff-accept path uses to decide whether to replace just the selection or
// the whole buffer. selActive is sufficient as the "has selection" predicate
// because the editor already clears it on no-op clicks and on cursor moves
// without shift, so a true value implies a non-empty range.
func (m *model) aiInputContent() (content string, onSelection bool) {
	if m.editor.selActive {
		return m.editor.SelectedText(), true
	}
	return m.editor.Value(), false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run TestAIInputContent`

Expected: PASS for both `TestAIInputContent_NoSelection` and `TestAIInputContent_WithSelection`.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`

Expected: all existing tests still pass; nothing regressed.

- [ ] **Step 6: Commit**

```bash
git add model.go plugin_selection_test.go
git commit -m "$(cat <<'EOF'
feat(model): add aiInputContent helper for AI-run inputs

Single read site for "send selection or whole buffer + did we scope to a selection?"

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Rename `shortcutOnSelection` → `aiRunOnSelection`

This is a mechanical rename across four files. No test changes; existing tests should still pass after.

**Files:**
- Modify: `model.go:132`
- Modify: `plugin_input.go:74,75`
- Modify: `plugin_diff.go:30,33,50`
- Modify: `shortcuts_input.go:44,45`

- [ ] **Step 1: Rename the declaration in `model.go`**

In `model.go`, find:
```go
	shortcutOnSelection      bool
```

Replace with:
```go
	aiRunOnSelection         bool
```

- [ ] **Step 2: Rename references in `plugin_input.go`**

In `plugin_input.go` find:
```go
				m.shortcutOnSelection = m.editor.selActive
				if m.shortcutOnSelection {
					content = m.editor.SelectedText()
				}
```

Replace with:
```go
				m.aiRunOnSelection = m.editor.selActive
				if m.aiRunOnSelection {
					content = m.editor.SelectedText()
				}
```

- [ ] **Step 3: Rename references in `plugin_diff.go`**

In `plugin_diff.go` find:
```go
	case "y":
		if m.shortcutOnSelection {
			// ReplaceSelection records its own op entry.
			m.editor.ReplaceSelection(m.pluginDiffResult)
			m.shortcutOnSelection = false
```

Replace with:
```go
	case "y":
		if m.aiRunOnSelection {
			// ReplaceSelection records its own op entry.
			m.editor.ReplaceSelection(m.pluginDiffResult)
			m.aiRunOnSelection = false
```

Then in the same file find:
```go
	case "n", "esc":
		m.shortcutOnSelection = false
```

Replace with:
```go
	case "n", "esc":
		m.aiRunOnSelection = false
```

- [ ] **Step 4: Rename references in `shortcuts_input.go`**

In `shortcuts_input.go` find:
```go
		content := m.editor.Value()
		m.shortcutOnSelection = m.editor.selActive
		if m.shortcutOnSelection {
			content = m.editor.SelectedText()
		}
```

Replace with:
```go
		content := m.editor.Value()
		m.aiRunOnSelection = m.editor.selActive
		if m.aiRunOnSelection {
			content = m.editor.SelectedText()
		}
```

- [ ] **Step 5: Verify no stale references remain**

Run: `grep -rn "shortcutOnSelection" .`

Expected: no matches.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add model.go plugin_input.go plugin_diff.go shortcuts_input.go
git commit -m "$(cat <<'EOF'
refactor: rename shortcutOnSelection to aiRunOnSelection

The flag now spans plugin and shortcut runs, so name it for the broader meaning.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Refactor `handleShortcutSelect` to use the helper, with regression test

The shortcut path already produces correct selection-scoped behavior. We're (a) adding a regression test that locks in that behavior, and (b) replacing the inline three-liner with the helper. No external behavior change.

**Files:**
- Modify: `shortcuts_input.go:43–47`
- Modify: `plugin_selection_test.go`

- [ ] **Step 1: Add a regression test to `plugin_selection_test.go`**

Open `plugin_selection_test.go`. Append:

```go
func TestShortcutSelect_WithSelection_SendsOnlySelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	provider := defaultAIShortcutProvider
	plugin := pluginByName(m.plugins, provider)
	if plugin == nil {
		// newTestModel passes nil plugins; install a fake one keyed by the
		// default provider so handleShortcutSelect's lookup succeeds.
		plugin = &fakePlugin{name: provider}
		m.plugins = []Plugin{plugin}
	}
	if err := savePluginConfig(provider, map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.shortcuts = []AIShortcut{{Name: "n", Description: "d", Prompt: "p"}}
	m.shortcutCursor = 0
	m.activeShortcutProvider = provider
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(pressEnter())
	nm := next.(model)

	if nm.pluginDiffOriginal != "hello" {
		t.Errorf("pluginDiffOriginal = %q, want %q", nm.pluginDiffOriginal, "hello")
	}
	if !nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = false, want true")
	}
}
```

This test references a `fakePlugin` we'll define in the same file. Append the fake plugin definition (it'll be reused by later tests):

```go
type fakePlugin struct {
	name string
}

func (f *fakePlugin) Name() string        { return f.name }
func (f *fakePlugin) Description() string { return "fake" }
func (f *fakePlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key"},
		{Key: "model", Label: "Model"},
	}
}
func (f *fakePlugin) Run(content, prompt string, config map[string]string) (string, error) {
	return "result", nil
}
```

- [ ] **Step 2: Run the new test**

Run: `go test ./... -run TestShortcutSelect_WithSelection_SendsOnlySelection -v`

Expected: PASS. The shortcut path is already correct; the test characterizes existing behavior.

- [ ] **Step 3: Refactor `handleShortcutSelect` to use the helper**

Open `shortcuts_input.go`. Find the block:
```go
		content := m.editor.Value()
		m.aiRunOnSelection = m.editor.selActive
		if m.aiRunOnSelection {
			content = m.editor.SelectedText()
		}
		m.pluginDiffOriginal = content
```

Replace with:
```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
```

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`

Expected: all tests pass, including the new regression test.

- [ ] **Step 5: Commit**

```bash
git add shortcuts_input.go plugin_selection_test.go
git commit -m "$(cat <<'EOF'
refactor(shortcuts): use aiInputContent helper for selection scoping

Adds regression test covering the existing selection-scoped shortcut path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Refactor `handlePluginConfig` shortcut-pending branch to use the helper

This is the third call site that already does the right thing inline. Switch it to the helper for consistency. Existing integration tests cover the surrounding behavior.

**Files:**
- Modify: `plugin_input.go:73–76` (inside `handlePluginConfig`)

- [ ] **Step 1: Refactor the branch**

Open `plugin_input.go`. Find:
```go
				shortcut := m.shortcuts[m.shortcutCursor]
				provider := m.pluginActive.Name()
				cfg, _ := loadPluginConfig(provider)
				content := m.editor.Value()
				m.aiRunOnSelection = m.editor.selActive
				if m.aiRunOnSelection {
					content = m.editor.SelectedText()
				}
				m.pluginDiffOriginal = content
```

Replace with:
```go
				shortcut := m.shortcuts[m.shortcutCursor]
				provider := m.pluginActive.Name()
				cfg, _ := loadPluginConfig(provider)
				content, onSelection := m.aiInputContent()
				m.aiRunOnSelection = onSelection
				m.pluginDiffOriginal = content
```

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add plugin_input.go
git commit -m "$(cat <<'EOF'
refactor(plugin): use aiInputContent helper in shortcut-pending config branch

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Fix `handlePluginPrompt` to be selection-aware (TDD)

This is the actual behavior change.

**Files:**
- Modify: `plugin_input.go:106–138` (`handlePluginPrompt`)
- Modify: `plugin_selection_test.go`

- [ ] **Step 1: Add the no-selection regression test (should pass before the fix)**

Open `plugin_selection_test.go`. Append:

```go
func TestPluginPrompt_NoSelection_SendsWholeContent(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	plugin := &fakePlugin{name: "fake"}
	m.pluginActive = plugin
	if err := savePluginConfig(plugin.Name(), map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.pluginPromptInput.SetValue("rewrite please")
	m.inputMode = inputPluginPrompt

	next, _ := m.handlePluginPrompt(pressEnter())
	nm := next.(model)

	if nm.pluginDiffOriginal != "hello world" {
		t.Errorf("pluginDiffOriginal = %q, want %q", nm.pluginDiffOriginal, "hello world")
	}
	if nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = true, want false")
	}
}
```

- [ ] **Step 2: Add the with-selection test (should fail before the fix)**

Append to `plugin_selection_test.go`:

```go
func TestPluginPrompt_WithSelection_SendsOnlySelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	plugin := &fakePlugin{name: "fake"}
	m.pluginActive = plugin
	if err := savePluginConfig(plugin.Name(), map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.pluginPromptInput.SetValue("rewrite please")
	m.inputMode = inputPluginPrompt

	next, _ := m.handlePluginPrompt(pressEnter())
	nm := next.(model)

	if nm.pluginDiffOriginal != "hello" {
		t.Errorf("pluginDiffOriginal = %q, want %q", nm.pluginDiffOriginal, "hello")
	}
	if !nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = false, want true")
	}
}
```

- [ ] **Step 3: Run the new tests to confirm the failure pattern**

Run: `go test ./... -run TestPluginPrompt -v`

Expected:
- `TestPluginPrompt_NoSelection_SendsWholeContent`: PASS
- `TestPluginPrompt_WithSelection_SendsOnlySelection`: FAIL with `pluginDiffOriginal = "hello world", want "hello"` (and `aiRunOnSelection = false, want true`)

This confirms the fix has actual work to do.

- [ ] **Step 4: Apply the fix to `handlePluginPrompt`**

Open `plugin_input.go`. Find inside `handlePluginPrompt`:
```go
		cfg, err := loadPluginConfig(m.pluginActive.Name())
		if err != nil {
			m.errMsg = "Failed to load plugin config: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		content := m.editor.Value()
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
```

Replace with:
```go
		cfg, err := loadPluginConfig(m.pluginActive.Name())
		if err != nil {
			m.errMsg = "Failed to load plugin config: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
```

- [ ] **Step 5: Run the new tests**

Run: `go test ./... -run TestPluginPrompt -v`

Expected: both `TestPluginPrompt_NoSelection_SendsWholeContent` and `TestPluginPrompt_WithSelection_SendsOnlySelection` PASS.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`

Expected: everything passes.

- [ ] **Step 7: Commit**

```bash
git add plugin_input.go plugin_selection_test.go
git commit -m "$(cat <<'EOF'
feat(plugin): scope custom-prompt runs to active selection

When the editor has an active selection, the plugin custom-prompt path
now sends only the selected text to the LLM and the diff/accept flow
replaces just that selection. Empty selection preserves whole-note
behavior. Adds tests covering both branches.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Diff-accept regression tests

The accept path (`y` in `handlePluginDiff`) already branches on `aiRunOnSelection`. Lock the behavior in with tests.

**Files:**
- Modify: `plugin_selection_test.go`

- [ ] **Step 1: Add the on-selection accept test**

Append to `plugin_selection_test.go`:

```go
func TestPluginDiffAccept_OnSelection_ReplacesOnlySelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	m.aiRunOnSelection = true
	m.pluginDiffOriginal = "hello"
	m.pluginDiffResult = "HOWDY"
	m.inputMode = inputPluginDiff

	next, _ := m.handlePluginDiff(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	nm := next.(model)

	if nm.editor.Value() != "HOWDY world" {
		t.Errorf("editor.Value() = %q, want %q", nm.editor.Value(), "HOWDY world")
	}
	if nm.aiRunOnSelection {
		t.Error("aiRunOnSelection should be reset to false after accept")
	}
	if nm.editor.selActive {
		t.Error("selection should be cleared after accept")
	}
}
```

This test imports `tea`. Add the import to the file's `import` block:

```go
import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

- [ ] **Step 2: Add the no-selection accept test**

Append to `plugin_selection_test.go`:

```go
func TestPluginDiffAccept_NoSelection_ReplacesWholeContent(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	m.aiRunOnSelection = false
	m.pluginDiffOriginal = "hello world"
	m.pluginDiffResult = "totally different"
	m.inputMode = inputPluginDiff

	next, _ := m.handlePluginDiff(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	nm := next.(model)

	if nm.editor.Value() != "totally different" {
		t.Errorf("editor.Value() = %q, want %q", nm.editor.Value(), "totally different")
	}
}
```

- [ ] **Step 3: Run the new tests**

Run: `go test ./... -run TestPluginDiffAccept -v`

Expected: both PASS.

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add plugin_selection_test.go
git commit -m "$(cat <<'EOF'
test(plugin): cover diff-accept for selection and whole-note paths

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Diff-reject regression test

The reject path (`n`/`esc` in `handlePluginDiff`) leaves the buffer unchanged, resets `aiRunOnSelection`, and preserves the active selection so the user can retry.

**Files:**
- Modify: `plugin_selection_test.go`

- [ ] **Step 1: Add the reject test**

Append to `plugin_selection_test.go`:

```go
func TestPluginDiffReject_OnSelection_LeavesContentUnchanged(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	m.aiRunOnSelection = true
	m.pluginDiffOriginal = "hello"
	m.pluginDiffResult = "HOWDY"
	m.inputMode = inputPluginDiff

	next, _ := m.handlePluginDiff(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	nm := next.(model)

	if nm.editor.Value() != "hello world" {
		t.Errorf("editor.Value() = %q, want %q (buffer should be unchanged)", nm.editor.Value(), "hello world")
	}
	if nm.aiRunOnSelection {
		t.Error("aiRunOnSelection should be reset to false after reject")
	}
	if !nm.editor.selActive {
		t.Error("selection should still be active after reject (so user can retry)")
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
}
```

- [ ] **Step 2: Run the new test**

Run: `go test ./... -run TestPluginDiffReject -v`

Expected: PASS.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add plugin_selection_test.go
git commit -m "$(cat <<'EOF'
test(plugin): cover diff-reject preserves buffer and selection

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: README updates

**Files:**
- Modify: `README.md` (Global keybindings table at line 80; AI Shortcuts paragraph at line 144)

- [ ] **Step 1: Add `Ctrl+G` row to the Global keybindings table**

Open `README.md`. Find:
```markdown
| `Ctrl+Space` | Open plugin selector |
```

Replace with:
```markdown
| `Ctrl+Space` | Open plugin selector |
| `Ctrl+G` | Open AI shortcut selector |
| `Ctrl+L` | Create AI shortcut |
```

(The help modal already lists `Ctrl+L`; the README was missing both.)

- [ ] **Step 2: Fix the AI Shortcuts paragraph**

Find:
```markdown
Quick text transformations powered by your configured LLM. Press `Ctrl+Space`, pick a shortcut, and the model rewrites or augments the current note. The diff view lets you accept or reject the change.
```

Replace with:
```markdown
Quick text transformations powered by your configured LLM. Press `Ctrl+G`, pick a shortcut, and the model rewrites or augments the current note. The diff view lets you accept or reject the change.

If text is selected when you trigger a plugin or shortcut, only the selected text is sent to the LLM and the diff/accept flow replaces just that selection. With no selection, the whole note is rewritten as before.
```

- [ ] **Step 3: Verify the file builds (no markdown lint required, just sanity)**

Run: `go test ./...`

Expected: all tests pass (no Go change, but confirm the working tree is healthy).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
docs: fix AI shortcut keybinding and document selection-scoped runs

Adds the missing Ctrl+G and Ctrl+L rows to the Global keybindings
table, corrects the AI Shortcuts paragraph that referenced Ctrl+Space,
and documents that an active selection scopes plugin/shortcut runs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

- [ ] **Run the full test suite one more time**

Run: `go test ./...`

Expected: all tests pass.

- [ ] **Build the binary**

Run: `go build ./...`

Expected: clean build.

- [ ] **Confirm git tree is clean**

Run: `git status`

Expected: `nothing to commit, working tree clean`.

- [ ] **Show the commits made**

Run: `git log --oneline -10`

Expected: eight new commits at the top: helper, rename, shortcut refactor, plugin-config refactor, plugin-prompt fix, accept tests, reject test, README.
