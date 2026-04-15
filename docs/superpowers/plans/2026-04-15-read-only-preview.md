# Read-Only Preview Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to scroll file previews without entering edit mode, with Tab switching focus between tree and preview.

**Architecture:** Reuse existing `activePanel`/`editorMode` state to express three focus modes (tree, preview, edit). Modify the Tab handler, expand preview key handling in `handleEditorKeys`, add a focused border style, and fix a rendering height bug.

**Tech Stack:** Go, Bubble Tea (charmbracelet/bubbletea), Lipgloss (charmbracelet/lipgloss), Bubbles viewport

---

## File Structure

- **Modify:** `model.go` — Tab handler, `handleEditorKeys` preview block, `View()` style selection
- **Modify:** `editor.go` — Add `previewFocusedStyle` definition
- **Modify:** `tree.go` — Fix height consistency in `View()` rendering

No new files created.

---

### Task 1: Fix Tab handler to skip editor.Focus() in preview mode

**Files:**
- Modify: `model.go:455-463`

The current Tab handler always calls `m.editor.Focus()` when switching to the editor panel. In preview mode, the textarea should NOT be focused — only the viewport should receive input.

- [ ] **Step 1: Modify the Tab handler**

In `model.go`, replace the Tab case (lines 455-463):

```go
		case "tab":
			if m.activePanel == treePanel {
				m.activePanel = editorPanel
				cmd := m.editor.Focus()
				return m, cmd
			}
			m.activePanel = treePanel
			m.editor.Blur()
			return m, nil
```

with:

```go
		case "tab":
			if m.activePanel == treePanel {
				m.activePanel = editorPanel
				if m.editorMode == modeEdit {
					cmd := m.editor.Focus()
					return m, cmd
				}
				return m, nil
			}
			m.activePanel = treePanel
			m.editor.Blur()
			return m, nil
```

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: Compiles with no errors.

- [ ] **Step 3: Commit**

```bash
git add model.go
git commit -m "fix(preview): skip editor.Focus() when Tab switches to preview mode"
```

---

### Task 2: Expand preview key handling in handleEditorKeys

**Files:**
- Modify: `model.go:540-570`

Currently `handleEditorKeys` has a shared Esc handler at the top (for both edit and preview), then a preview block that only passes keys to the viewport. The Esc handler does edit-mode things (ClearSelection, isDirty guard) that don't apply to preview. This needs to be restructured: preview mode gets its own Esc/Enter/Right/printable-key handling before the viewport pass-through.

- [ ] **Step 1: Rewrite handleEditorKeys**

In `model.go`, replace the entire `handleEditorKeys` function (lines 540-570):

```go
func (m model) handleEditorKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.editor.ClearSelection()
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingSwitchFile
			m.pendingSwitchPath = m.currentFile
			return m, nil
		}
		m.activePanel = treePanel
		m.editor.Blur()
		// Switch to preview mode so the note shows as read-only
		if m.currentFile != "" {
			content := m.editor.Value()
			vp := viewport.New(m.editorWidth-2, m.editorHeight)
			vp.SetContent(wordWrap(content, m.editorWidth-4))
			m.preview = vp
			m.editorMode = modePreview
		}
		return m, nil
	}

	if m.editorMode == modePreview {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}

	cmd := m.editor.HandleKey(msg)
	return m, cmd
}
```

with:

```go
func (m model) handleEditorKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editorMode == modePreview {
		switch msg.String() {
		case "esc":
			m.activePanel = treePanel
			return m, nil
		case "enter", "right":
			m.editorMode = modeEdit
			cmd := m.editor.Focus()
			return m, cmd
		default:
			if msg.Type == tea.KeyRunes {
				m.editorMode = modeEdit
				m.editor.Focus()
				cmd := m.editor.HandleKey(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}
	}

	// Edit mode
	if msg.String() == "esc" {
		m.editor.ClearSelection()
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingSwitchFile
			m.pendingSwitchPath = m.currentFile
			return m, nil
		}
		m.activePanel = treePanel
		m.editor.Blur()
		if m.currentFile != "" {
			content := m.editor.Value()
			vp := viewport.New(m.editorWidth-2, m.editorHeight)
			vp.SetContent(wordWrap(content, m.editorWidth-4))
			m.preview = vp
			m.editorMode = modePreview
		}
		return m, nil
	}

	cmd := m.editor.HandleKey(msg)
	return m, cmd
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: Compiles with no errors.

- [ ] **Step 3: Run tests**

Run: `go test ./...`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add model.go
git commit -m "feat(preview): add Esc/Enter/Right/typing key handling in preview mode"
```

---

### Task 3: Add previewFocusedStyle and update View()

**Files:**
- Modify: `editor.go:8-9`
- Modify: `model.go:1160-1164`

Add a styled border on the preview panel when it has focus, and adjust viewport width to account for the border.

- [ ] **Step 1: Add previewFocusedStyle in editor.go**

In `editor.go`, after the `editorStyle` definition (line 8), add:

```go
var previewFocusedStyle = lipgloss.NewStyle().
	Padding(0, 1).
	BorderLeft(true).
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("117"))
```

- [ ] **Step 2: Update View() to use previewFocusedStyle**

In `model.go`, replace the preview rendering block (lines 1160-1164):

```go
	} else if m.editorMode == modePreview {
		rightView = previewStyle.
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(m.preview.View())
```

with:

```go
	} else if m.editorMode == modePreview {
		style := previewStyle
		if m.activePanel == editorPanel {
			style = previewFocusedStyle
		}
		rightView = style.
			Width(m.editorWidth).
			Height(m.editorHeight).
			Render(m.preview.View())
```

- [ ] **Step 3: Build and verify**

Run: `go build ./...`
Expected: Compiles with no errors.

- [ ] **Step 4: Commit**

```bash
git add editor.go model.go
git commit -m "feat(preview): add highlighted left border when preview is focused"
```

---

### Task 4: Fix tree scroll bug

**Files:**
- Modify: `tree.go:201`

The tree panel uses `MaxHeight(tp.height)` which caps output but doesn't guarantee a fixed height. The preview panel uses `Height(m.editorHeight)` which forces an exact height. When `JoinHorizontal` combines panels with different heights, the shorter panel can shift. Fix by using `Height` instead of `MaxHeight` on the tree panel to guarantee consistent rendered height.

- [ ] **Step 1: Change MaxHeight to Height in tree.go**

In `tree.go`, replace line 201:

```go
	return treePanelStyle.Width(tp.width).MaxHeight(tp.height).Render(rendered)
```

with:

```go
	return treePanelStyle.Width(tp.width).Height(tp.height).Render(rendered)
```

- [ ] **Step 2: Build and verify**

Run: `go build ./...`
Expected: Compiles with no errors.

- [ ] **Step 3: Manual test — reproduce the original bug**

Run: `go run . ~/Notes/Notes`

1. Navigate to a file with long content (e.g., a task file with wrapped lines)
2. Press Tab to focus the preview
3. Scroll down with arrow keys or pgdn
4. Verify: the tree panel stays fixed and does not scroll

If the tree still shifts, the root cause is elsewhere — investigate the lipgloss `Width()` accounting:
- Check if `treePanelStyle.Width(tp.width)` includes or excludes padding and border
- Verify total rendered width of tree + preview does not exceed `m.width`
- Add debug output: `fmt.Fprintf(os.Stderr, "tree lines: %d, preview lines: %d\n", strings.Count(treeView, "\n")+1, strings.Count(rightView, "\n")+1)` in `View()` to compare actual heights

- [ ] **Step 4: Commit**

```bash
git add tree.go
git commit -m "fix(tree): use Height instead of MaxHeight to prevent scroll drift"
```

---

### Task 5: End-to-end manual testing

No code changes — verification only.

- [ ] **Step 1: Run the app**

Run: `go run . ~/Notes/Notes`

- [ ] **Step 2: Test tree → preview transition**

1. Navigate the file tree with j/k to select a file (preview appears)
2. Press Tab — verify the preview panel gets a colored left border
3. Press up/down — verify the preview scrolls, tree does NOT scroll
4. Press Esc — verify focus returns to the tree (border disappears), tree cursor is visible

- [ ] **Step 3: Test preview → edit transition**

1. Tab into preview
2. Press Enter — verify edit mode activates (cursor appears, you can type)
3. Press Esc — verify it goes back to tree with preview shown
4. Tab into preview again
5. Type a character — verify edit mode activates and the character is inserted
6. Press Esc — verify unsaved guard dialog appears (content is dirty)

- [ ] **Step 4: Test preview → edit via Right arrow**

1. Tab into preview
2. Press Right — verify edit mode activates

- [ ] **Step 5: Test Tab from edit mode**

1. While in edit mode, press Tab — verify focus returns to tree, editor blurs

- [ ] **Step 6: Run all tests**

Run: `go test ./...`
Expected: All tests pass.
