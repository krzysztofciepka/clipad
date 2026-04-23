# Mouse Support & Faster Text Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add mouse support (click, drag-select, wheel) in the editor and file tree, verify Home/End line navigation, and add a `Ctrl+Y` keybinding for manual git sync — without disturbing the existing keyboard flow.

**Architecture:** Pure coordinate helpers in a new `mouse.go`. Drag-selection re-uses the existing `selActive` / `selAnchor*` state so keyboard-selected and mouse-selected text share the same delete/copy/cut code path. Bubble Tea is enabled for `WithMouseCellMotion` so motion events only arrive while a button is held.

**Tech Stack:** Go, Bubble Tea `v1.3.10`, Bubbles `v1.0.0`, Lipgloss. Existing test style: table-driven tests in `*_test.go` files next to the code.

**Spec:** `docs/superpowers/specs/2026-04-23-mouse-and-text-edit-design.md`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `mouse.go` (new) | Pure coordinate helpers (`hitTestPanel`, `mousePosToEditorCursor`, `mousePosToTreeRow`, `editorNumWidth`) and mouse dispatcher (`handleMouseMsg`, `handleEditorMouse`, `handleTreeMouse`). |
| `selection.go` (modify) | Add `mouseDragging` field to `SelectableEditor`. Add `StartMouseDrag`, `UpdateMouseDrag`, `EndMouseDrag`, `ScrollUp`, `ScrollDown` methods. Existing `HandleKey` / selection logic unchanged. |
| `main.go` (modify) | Enable `tea.WithMouseCellMotion()` in the program options. |
| `model.go` (modify) | Add `case tea.MouseMsg:` to `Update`. Add `case "ctrl+y":` to the main key switch. |
| `git_sync.go` (modify) | Add `(m model) triggerManualGitSync()` method. |
| `README.md` (modify) | Document `Ctrl+Y` under Global keybindings; add a new "Mouse" section. |
| `mouse_test.go` (new) | Tests for pure helpers and the dispatcher. |
| `selection_test.go` (modify) | Tests for new mouse-drag methods, Home/End behavior, and backspace-after-selection. |
| `git_sync_test.go` (modify) | Tests for `triggerManualGitSync` (skip-if-running, no-remote-prompt, config-error). |

---

## Task 1: Pure mouse helpers and mouseDragging field

**Files:**
- Create: `mouse.go`
- Modify: `selection.go:18-26`
- Create: `mouse_test.go`

- [ ] **Step 1: Write failing tests for `hitTestPanel`**

Create `mouse_test.go`:

```go
package main

import (
	"testing"
)

func TestHitTestPanel(t *testing.T) {
	tests := []struct {
		name               string
		treeWidth, width, height, x, y int
		wantHit            panel
		wantLocalX, wantLocalY int
		wantOK             bool
	}{
		{"tree area", 20, 100, 30, 5, 5, treePanel, 5, 5, true},
		{"border column rejected", 20, 100, 30, 20, 5, 0, 0, 0, false},
		{"editor area", 20, 100, 30, 25, 5, editorPanel, 4, 5, true},
		{"status bar row rejected", 20, 100, 30, 5, 29, 0, 0, 0, false},
		{"out of bounds negative", 20, 100, 30, -1, 5, 0, 0, 0, false},
		{"out of bounds right", 20, 100, 30, 100, 5, 0, 0, 0, false},
		{"out of bounds below", 20, 100, 30, 5, 30, 0, 0, 0, false},
		{"narrow terminal treats all as editor", 0, 20, 30, 5, 5, editorPanel, 5, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, lx, ly, ok := hitTestPanel(tt.treeWidth, tt.width, tt.height, tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if hit != tt.wantHit || lx != tt.wantLocalX || ly != tt.wantLocalY {
				t.Errorf("hit=%v local=(%d,%d), want %v (%d,%d)",
					hit, lx, ly, tt.wantHit, tt.wantLocalX, tt.wantLocalY)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./... -run TestHitTestPanel
```

Expected: compilation error — `hitTestPanel` undefined.

- [ ] **Step 3: Write failing test for `mousePosToEditorCursor`**

Append to `mouse_test.go`:

```go
func TestMousePosToEditorCursor(t *testing.T) {
	content := "hello\nworld\nfoo bar"
	tests := []struct {
		name                             string
		viewOffset, localX, localY, numWidth int
		wantLine, wantCol                int
	}{
		{"first line first char", 0, 4, 0, 2, 0, 0},   // padding(1)+numWidth(2)+space(1)=4 → col 0
		{"first line middle", 0, 6, 0, 2, 0, 2},       // col = 6-2-2 = 2
		{"past line length clamps", 0, 20, 0, 2, 0, 5},
		{"second line", 0, 4, 1, 2, 1, 0},
		{"viewOffset shifts", 1, 4, 0, 2, 1, 0},
		{"past content clamps to last line", 0, 4, 99, 2, 2, 0},
		{"click in line number column", 0, 2, 0, 2, 0, 0},
		{"click in padding", 0, 0, 0, 2, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := mousePosToEditorCursor(content, tt.viewOffset, tt.localX, tt.localY, tt.numWidth)
			if line != tt.wantLine || col != tt.wantCol {
				t.Errorf("got (%d,%d), want (%d,%d)", line, col, tt.wantLine, tt.wantCol)
			}
		})
	}
}

func TestMousePosToTreeRow(t *testing.T) {
	if got := mousePosToTreeRow(0, 5); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
	if got := mousePosToTreeRow(10, 3); got != 13 {
		t.Errorf("got %d, want 13", got)
	}
}

func TestEditorNumWidth(t *testing.T) {
	tests := []struct {
		content string
		want    int
	}{
		{"single line", 2},
		{"one\ntwo\nthree", 2},
		{strings.Repeat("line\n", 99), 2}, // 100 lines → 3-digit max, but numWidth clamped to at least 2 only
		{strings.Repeat("line\n", 100), 3},
	}
	for _, tt := range tests {
		if got := editorNumWidth(tt.content); got != tt.want {
			t.Errorf("editorNumWidth(%d lines) = %d, want %d",
				strings.Count(tt.content, "\n")+1, got, tt.want)
		}
	}
}
```

Also add `"strings"` import at the top of `mouse_test.go`.

- [ ] **Step 4: Run the tests to verify they fail**

```bash
go test ./... -run "TestHitTestPanel|TestMousePosToEditorCursor|TestMousePosToTreeRow|TestEditorNumWidth"
```

Expected: compilation errors — all four functions undefined.

- [ ] **Step 5: Implement the pure helpers**

Create `mouse.go`:

```go
package main

import (
	"fmt"
	"strings"
)

// hitTestPanel maps a terminal mouse coordinate to the panel it landed in.
// The status-bar row and any out-of-bounds coordinates return ok=false.
// When treeWidth == 0 (narrow terminal), the full width is treated as editor.
func hitTestPanel(treeWidth, width, height, x, y int) (hit panel, localX, localY int, ok bool) {
	if x < 0 || y < 0 || x >= width || y >= height {
		return 0, 0, 0, false
	}
	if y >= height-1 {
		return 0, 0, 0, false
	}
	if treeWidth == 0 {
		return editorPanel, x, y, true
	}
	if x < treeWidth {
		return treePanel, x, y, true
	}
	if x == treeWidth {
		return 0, 0, 0, false
	}
	return editorPanel, x - treeWidth - 1, y, true
}

// editorNumWidth returns the width of the line-number column used by
// renderWithSelection, matching its formatting rules.
func editorNumWidth(content string) int {
	lines := strings.Split(content, "\n")
	w := len(fmt.Sprintf("%d", len(lines)))
	if w < 2 {
		w = 2
	}
	return w
}

// mousePosToEditorCursor translates panel-local coordinates to a (line, col)
// position in the editor content. Accounts for editorStyle's Padding(0, 1)
// left padding and the line-number column plus its trailing space.
func mousePosToEditorCursor(content string, viewOffset, localX, localY, numWidth int) (line, col int) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	line = viewOffset + localY
	if line < 0 {
		line = 0
	}
	if line > len(lines)-1 {
		line = len(lines) - 1
	}
	col = localX - numWidth - 2
	if col < 0 {
		col = 0
	}
	runes := []rune(lines[line])
	if col > len(runes) {
		col = len(runes)
	}
	return line, col
}

// mousePosToTreeRow translates local Y in the tree panel to an absolute row
// index. Caller must validate against len(items).
func mousePosToTreeRow(treeOffset, localY int) int {
	return treeOffset + localY
}
```

- [ ] **Step 6: Run the tests and verify they pass**

```bash
go test ./... -run "TestHitTestPanel|TestMousePosToEditorCursor|TestMousePosToTreeRow|TestEditorNumWidth"
```

Expected: PASS.

- [ ] **Step 7: Add `mouseDragging` field to `SelectableEditor`**

In `selection.go` around line 18-26, modify the struct:

```go
type SelectableEditor struct {
	textarea.Model
	height        int
	selActive     bool
	selAnchorLine int
	selAnchorCol  int
	textClip      string
	viewOffset    int
	mouseDragging bool
}
```

- [ ] **Step 8: Run the full build to verify no breakage**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 9: Commit**

```bash
git add mouse.go mouse_test.go selection.go
git commit -m "feat(mouse): add pure coordinate helpers and mouseDragging field

Introduces hitTestPanel, mousePosToEditorCursor, mousePosToTreeRow, and
editorNumWidth helpers used by the upcoming mouse dispatcher. Adds the
mouseDragging field to SelectableEditor in preparation for click/drag
selection handling."
```

---

## Task 2: SelectableEditor mouse-drag and scroll methods

**Files:**
- Modify: `selection.go`
- Modify: `selection_test.go`

- [ ] **Step 1: Write failing test for `StartMouseDrag` / `UpdateMouseDrag` / `EndMouseDrag`**

Append to `selection_test.go`:

```go
func TestMouseDragSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world\nsecond line")
	setEditorSize(&e, 80, 10)

	// Simulate press at (0, 3) — cursor to line 0, col 3
	e.StartMouseDrag(0, 3)
	if e.selActive {
		t.Error("selActive should not be set on press without motion")
	}
	if e.Line() != 0 {
		t.Errorf("line after press = %d, want 0", e.Line())
	}

	// Motion to (0, 8) — selection should activate
	e.UpdateMouseDrag(0, 8)
	if !e.selActive {
		t.Error("selActive should be true after motion to different position")
	}
	if e.selAnchorLine != 0 || e.selAnchorCol != 3 {
		t.Errorf("anchor = (%d,%d), want (0,3)", e.selAnchorLine, e.selAnchorCol)
	}
	got := e.SelectedText()
	if got != "lo wo" {
		t.Errorf("SelectedText = %q, want %q", got, "lo wo")
	}

	// Release — selection persists (cursor != anchor)
	e.EndMouseDrag()
	if !e.selActive {
		t.Error("selActive should persist after release with non-empty selection")
	}
	if e.mouseDragging {
		t.Error("mouseDragging should be false after release")
	}
}

func TestMouseClickWithoutDragClearsSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	// Press, no motion, release — should behave as a simple click.
	e.StartMouseDrag(0, 5)
	e.EndMouseDrag()
	if e.selActive {
		t.Error("selActive should be false after press+release without motion")
	}
	if e.mouseDragging {
		t.Error("mouseDragging should be false after release")
	}
}

func TestMouseDragBackToAnchorClearsSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	e.StartMouseDrag(0, 3)
	e.UpdateMouseDrag(0, 8) // activate selection
	e.UpdateMouseDrag(0, 3) // drag back to anchor
	e.EndMouseDrag()
	if e.selActive {
		t.Error("selActive should be false after dragging back to anchor")
	}
}

func TestEditorScroll(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("line1\nline2\nline3\nline4\nline5")
	setEditorSize(&e, 80, 3)

	// Move to line 3 (0-indexed), then scroll up by 2
	e.StartMouseDrag(3, 0)
	e.EndMouseDrag()
	e.ScrollUp(2)
	if e.Line() != 1 {
		t.Errorf("line after ScrollUp(2) = %d, want 1", e.Line())
	}

	e.ScrollDown(2)
	if e.Line() != 3 {
		t.Errorf("line after ScrollDown(2) = %d, want 3", e.Line())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./... -run "TestMouseDragSelection|TestMouseClickWithoutDragClearsSelection|TestMouseDragBackToAnchorClearsSelection|TestEditorScroll"
```

Expected: compilation error — `StartMouseDrag`, `UpdateMouseDrag`, `EndMouseDrag`, `ScrollUp`, `ScrollDown` undefined.

- [ ] **Step 3: Implement the methods**

Append to `selection.go` (after `SelectAll`, before `moveTo`):

```go
// StartMouseDrag positions the cursor at (line, col) and records the click as
// a potential selection anchor. Selection is not yet active — only a
// subsequent UpdateMouseDrag to a different position activates it.
func (e *SelectableEditor) StartMouseDrag(line, col int) {
	e.moveTo(line, col)
	e.selAnchorLine = line
	e.selAnchorCol = col
	e.selActive = false
	e.mouseDragging = true
}

// UpdateMouseDrag moves the cursor during a drag. The first position that
// differs from the anchor activates the selection.
func (e *SelectableEditor) UpdateMouseDrag(line, col int) {
	if !e.mouseDragging {
		return
	}
	e.moveTo(line, col)
	if line != e.selAnchorLine || col != e.selAnchorCol {
		e.selActive = true
	} else {
		e.selActive = false
	}
	e.adjustViewOffset()
}

// EndMouseDrag finishes a drag. Clears mouseDragging. If the cursor is still
// at the anchor (no drag happened), selection is cleared.
func (e *SelectableEditor) EndMouseDrag() {
	e.mouseDragging = false
	if e.Line() == e.selAnchorLine && e.cursorCol() == e.selAnchorCol {
		e.ClearSelection()
	}
}

// ScrollUp moves cursor and viewport up by n lines.
func (e *SelectableEditor) ScrollUp(n int) {
	for i := 0; i < n; i++ {
		e.moveCursorUp()
	}
	e.adjustViewOffset()
}

// ScrollDown moves cursor and viewport down by n lines.
func (e *SelectableEditor) ScrollDown(n int) {
	for i := 0; i < n; i++ {
		e.moveCursorDown()
	}
	e.adjustViewOffset()
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
go test ./... -run "TestMouseDragSelection|TestMouseClickWithoutDragClearsSelection|TestMouseDragBackToAnchorClearsSelection|TestEditorScroll"
```

Expected: PASS.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all tests PASS — no regression in existing selection tests.

- [ ] **Step 6: Commit**

```bash
git add selection.go selection_test.go
git commit -m "feat(mouse): add drag-selection and scroll methods on editor

StartMouseDrag / UpdateMouseDrag / EndMouseDrag wrap the anchor and
selActive state so a simple click positions the cursor without creating
a flashing empty selection. ScrollUp and ScrollDown move both cursor
and viewport to stay consistent with bubbles textarea's internal
scroll tracking."
```

---

## Task 3: handleEditorMouse dispatcher

**Files:**
- Modify: `mouse.go`
- Modify: `mouse_test.go`

- [ ] **Step 1: Write failing test for editor click/drag/wheel via `handleEditorMouse`**

Append to `mouse_test.go`:

```go
import tea "github.com/charmbracelet/bubbletea"
```

(Add the import at the top of the file alongside the existing imports.)

```go
func newMouseTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "")
	m.width = 100
	m.height = 30
	m.treeWidth = 20
	m.editorWidth = 79
	m.editorHeight = 29
	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)
	return m
}

func TestHandleEditorMouse_PressPositionsCursor(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello world\nsecond line")
	m.activePanel = treePanel

	// Editor begins at x = treeWidth+1 = 21. Click at local (5, 0) =
	// absolute x=26, y=0. Local X 5 → col = 5-2-2 = 1.
	msg := tea.MouseMsg{X: 26, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, msg)
	nm := next.(model)
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", nm.activePanel)
	}
	if nm.editor.Line() != 0 || nm.editor.cursorCol() != 1 {
		t.Errorf("cursor = (%d,%d), want (0,1)", nm.editor.Line(), nm.editor.cursorCol())
	}
}

func TestHandleEditorMouse_DragSelects(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")

	// Press at local (5, 0) → col 1
	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, press)
	m = next.(model)

	// Motion to local (9, 0) → col 5
	motion := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion}
	next, _ = handleEditorMouse(m, 9, 0, motion)
	m = next.(model)

	if !m.editor.selActive {
		t.Error("selActive should be true after motion")
	}
	got := m.editor.SelectedText()
	if got != "ello" {
		t.Errorf("SelectedText = %q, want %q", got, "ello")
	}

	release := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	next, _ = handleEditorMouse(m, 9, 0, release)
	m = next.(model)
	if !m.editor.selActive {
		t.Error("selActive should persist after release with selection")
	}
}

func TestHandleEditorMouse_WheelScrolls(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("line1\nline2\nline3\nline4\nline5")
	m.editor.StartMouseDrag(3, 0)
	m.editor.EndMouseDrag()

	wheel := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 0, 0, wheel)
	m = next.(model)
	if m.editor.Line() != 0 {
		t.Errorf("line after wheel up = %d, want 0", m.editor.Line())
	}

	wheelDown := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	next, _ = handleEditorMouse(m, 0, 0, wheelDown)
	m = next.(model)
	if m.editor.Line() != 3 {
		t.Errorf("line after wheel down = %d, want 3", m.editor.Line())
	}
}

func TestHandleEditorMouse_PreviewModeClickFocusesEdit(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello")
	m.editorMode = modePreview
	m.activePanel = editorPanel

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, press)
	m = next.(model)
	if m.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", m.editorMode)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./... -run "TestHandleEditorMouse"
```

Expected: compilation error — `handleEditorMouse` undefined.

- [ ] **Step 3: Implement `handleEditorMouse`**

Append to `mouse.go`:

```go
import tea "github.com/charmbracelet/bubbletea"
```

(Update the imports at the top of `mouse.go` to include the `tea` import alongside `fmt` and `strings`.)

```go
// handleEditorMouse routes a mouse event that landed in the editor panel.
// localX/localY are relative to the editor panel's top-left.
func handleEditorMouse(m model, localX, localY int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		numWidth := editorNumWidth(m.editor.Value())
		line, col := mousePosToEditorCursor(m.editor.Value(), m.editor.viewOffset, localX, localY, numWidth)
		switch msg.Action {
		case tea.MouseActionPress:
			m.activePanel = editorPanel
			if m.editorMode == modePreview {
				m.editorMode = modeEdit
				m.editor.Focus()
			}
			m.editor.StartMouseDrag(line, col)
		case tea.MouseActionMotion:
			m.editor.UpdateMouseDrag(line, col)
		case tea.MouseActionRelease:
			m.editor.EndMouseDrag()
		}
		return m, nil
	case tea.MouseButtonWheelUp:
		m.editor.ScrollUp(3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.editor.ScrollDown(3)
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
go test ./... -run "TestHandleEditorMouse"
```

Expected: PASS.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add mouse.go mouse_test.go
git commit -m "feat(mouse): route editor clicks, drags, and wheel events

handleEditorMouse translates panel-local coordinates via
mousePosToEditorCursor and delegates to the new SelectableEditor
drag/scroll methods. Clicks in preview mode flip the editor back to
edit mode; clicks while the tree is focused steal focus."
```

---

## Task 4: handleTreeMouse dispatcher

**Files:**
- Modify: `mouse.go`
- Modify: `mouse_test.go`

- [ ] **Step 1: Write failing test for tree click and wheel**

Append to `mouse_test.go`:

```go
func newMouseTreeModel(t *testing.T) model {
	t.Helper()
	m := newMouseTestModel(t)
	vault := m.vault
	os.WriteFile(filepath.Join(vault, "alpha.md"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(vault, "beta.md"), []byte("beta"), 0o644)
	os.Mkdir(filepath.Join(vault, "sub"), 0o755)
	os.WriteFile(filepath.Join(vault, "sub", "c.md"), []byte("c"), 0o644)
	m.refreshTree()
	m.tree.height = 10
	m.tree.width = 20
	return m
}

func TestHandleTreeMouse_ClickFileSelectsAndPreviews(t *testing.T) {
	m := newMouseTreeModel(t)
	// Row 0 is "sub" (directories first), row 1 is "alpha.md"
	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleTreeMouse(m, 1, press)
	m = next.(model)
	if m.tree.cursor != 1 {
		t.Errorf("tree.cursor = %d, want 1", m.tree.cursor)
	}
	if m.currentFile == "" {
		t.Error("currentFile should be set after clicking a file")
	}
	if m.editorMode != modePreview {
		t.Errorf("editorMode = %v, want modePreview", m.editorMode)
	}
	if m.activePanel != treePanel {
		t.Errorf("activePanel = %v, want treePanel", m.activePanel)
	}
}

func TestHandleTreeMouse_ClickFolderToggles(t *testing.T) {
	m := newMouseTreeModel(t)
	// Row 0 = "sub" (directory)
	node := m.tree.items[0].Node
	if !node.IsDir {
		t.Fatalf("expected row 0 to be a directory; got %+v", node)
	}
	initialExpanded := node.Expanded

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleTreeMouse(m, 0, press)
	m = next.(model)
	if m.tree.items[0].Node.Expanded == initialExpanded {
		t.Error("folder expanded state should have toggled")
	}
}

func TestHandleTreeMouse_WheelScrolls(t *testing.T) {
	m := newMouseTreeModel(t)
	m.tree.offset = 0
	wheel := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	// Requires enough rows for scroll to register; harmless if clamped.
	next, _ := handleTreeMouse(m, 0, wheel)
	m = next.(model)
	// offset either increased or was clamped by clampOffset.
	if m.tree.offset < 0 {
		t.Errorf("offset went negative: %d", m.tree.offset)
	}

	m.tree.offset = 5
	wheelUp := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}
	next, _ = handleTreeMouse(m, 0, wheelUp)
	m = next.(model)
	if m.tree.offset > 2 {
		t.Errorf("offset after wheel up = %d, want <= 2", m.tree.offset)
	}
}

func TestHandleTreeMouse_OutOfBoundsRowIgnored(t *testing.T) {
	m := newMouseTreeModel(t)
	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	before := m.tree.cursor
	next, _ := handleTreeMouse(m, 99, press) // no row 99
	m = next.(model)
	if m.tree.cursor != before {
		t.Errorf("cursor moved unexpectedly: before=%d after=%d", before, m.tree.cursor)
	}
}
```

Add to the `mouse_test.go` imports: `"os"`, `"path/filepath"`.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./... -run "TestHandleTreeMouse"
```

Expected: compilation error — `handleTreeMouse` undefined.

- [ ] **Step 3: Implement `handleTreeMouse`**

Append to `mouse.go`:

```go
// handleTreeMouse routes a mouse event that landed in the tree panel.
func handleTreeMouse(m model, localY int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		row := mousePosToTreeRow(m.tree.offset, localY)
		if row < 0 || row >= len(m.tree.items) {
			return m, nil
		}
		m.tree.cursor = row
		m.tree.clampOffset()
		m.activePanel = treePanel
		node := m.tree.items[row].Node
		if node.IsDir {
			node.Expanded = !node.Expanded
			m.tree.rebuildItems()
		} else {
			m.previewSelectedFile()
		}
		return m, nil
	case tea.MouseButtonWheelUp:
		m.tree.offset -= 3
		if m.tree.offset < 0 {
			m.tree.offset = 0
		}
		m.tree.clampOffset()
		return m, nil
	case tea.MouseButtonWheelDown:
		m.tree.offset += 3
		m.tree.clampOffset()
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
go test ./... -run "TestHandleTreeMouse"
```

Expected: PASS.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add mouse.go mouse_test.go
git commit -m "feat(mouse): route tree clicks and wheel events

Clicking a file moves the tree cursor and opens the file in preview
mode via the existing previewSelectedFile path. Clicking a folder
toggles its expanded state. Wheel up/down scrolls tp.offset and
clamps."
```

---

## Task 5: Mouse dispatcher and wire-up

**Files:**
- Modify: `mouse.go`
- Modify: `main.go:128`
- Modify: `model.go` (inside `Update`)
- Modify: `mouse_test.go`

- [ ] **Step 1: Write failing test for dispatcher routing**

Append to `mouse_test.go`:

```go
func TestHandleMouseMsg_StatusBarIgnored(t *testing.T) {
	m := newMouseTestModel(t)
	before := m.activePanel
	msg := tea.MouseMsg{X: 10, Y: 29, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, cmd := handleMouseMsg(m, msg)
	m = next.(model)
	if cmd != nil {
		t.Error("status-bar click should return nil cmd")
	}
	if m.activePanel != before {
		t.Error("status-bar click should not change active panel")
	}
}

func TestHandleMouseMsg_PreviewWheelForwardsToViewport(t *testing.T) {
	m := newMouseTestModel(t)
	m.editorMode = modePreview
	m.activePanel = editorPanel
	msg := tea.MouseMsg{X: 50, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	// Simply check we don't panic and state is preserved (viewport handles the scroll internally).
	next, _ := handleMouseMsg(m, msg)
	m = next.(model)
	if m.editorMode != modePreview {
		t.Error("preview wheel should not change editorMode")
	}
}

func TestHandleMouseMsg_RoutesToEditor(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello")
	// Absolute X=25 with treeWidth=20 and border at 20 → editor localX = 25-20-1 = 4
	msg := tea.MouseMsg{X: 25, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleMouseMsg(m, msg)
	m = next.(model)
	if m.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", m.activePanel)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./... -run "TestHandleMouseMsg"
```

Expected: compilation error — `handleMouseMsg` undefined.

- [ ] **Step 3: Implement `handleMouseMsg`**

Append to `mouse.go`:

```go
// handleMouseMsg is the top-level mouse dispatcher. Callers must ensure
// m.inputMode == inputNone and !m.pluginProcessing before invoking.
func handleMouseMsg(m model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	hit, localX, localY, ok := hitTestPanel(m.treeWidth, m.width, m.height, msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	switch hit {
	case treePanel:
		return handleTreeMouse(m, localY, msg)
	case editorPanel:
		if m.editorMode == modePreview && m.activePanel == editorPanel &&
			(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			var cmd tea.Cmd
			m.preview, cmd = m.preview.Update(msg)
			return m, cmd
		}
		return handleEditorMouse(m, localX, localY, msg)
	}
	return m, nil
}
```

- [ ] **Step 4: Run the dispatcher tests and verify they pass**

```bash
go test ./... -run "TestHandleMouseMsg"
```

Expected: PASS.

- [ ] **Step 5: Enable mouse in the program**

In `main.go:128`, modify:

```go
p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
```

- [ ] **Step 6: Wire the MouseMsg case in `model.Update`**

In `model.go`, inside `Update`, add a new case to the outer type-switch. Place it right before the `case tea.KeyMsg:` branch (around line 357):

```go
case tea.MouseMsg:
	if m.pluginProcessing {
		return m, nil
	}
	if m.inputMode != inputNone {
		return m, nil
	}
	return handleMouseMsg(m, msg)
```

- [ ] **Step 7: Build and run the full test suite**

```bash
go build ./... && go test ./...
```

Expected: build success, all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add mouse.go mouse_test.go main.go model.go
git commit -m "feat(mouse): enable mouse events and wire dispatcher

Bubble Tea now starts with WithMouseCellMotion and model.Update routes
tea.MouseMsg through handleMouseMsg. Status-bar clicks and clicks during
modal input modes are ignored; preview-mode wheel events forward to the
viewport viewport for native scroll."
```

---

## Task 6: Home/End and delete-selection verification tests

**Files:**
- Modify: `selection_test.go`

- [ ] **Step 1: Write failing tests for Home/End + delete-selection integration**

Append to `selection_test.go`:

```go
func TestHandleKey_DeleteSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	// Select "hello" via shift+right from position 0
	for i := 0; i < 5; i++ {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	if !e.selActive {
		t.Fatal("shift+right should activate selection")
	}

	// Backspace deletes the selection.
	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if e.Value() != " world" {
		t.Errorf("after delete-selection: Value = %q, want %q", e.Value(), " world")
	}
	if e.selActive {
		t.Error("selection should be cleared after delete")
	}
}

func TestHandleKey_DeleteMouseSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	// Mouse-select "hello"
	e.StartMouseDrag(0, 0)
	e.UpdateMouseDrag(0, 5)
	e.EndMouseDrag()

	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if e.Value() != " world" {
		t.Errorf("after delete-mouse-selection: Value = %q, want %q", e.Value(), " world")
	}
}

func TestHandleKey_HomeEndMoveCursor(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)
	// Move cursor to middle of line via right arrow
	for i := 0; i < 5; i++ {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyHome})
	if e.cursorCol() != 0 {
		t.Errorf("after Home: col = %d, want 0", e.cursorCol())
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyEnd})
	if e.cursorCol() != len("hello world") {
		t.Errorf("after End: col = %d, want %d", e.cursorCol(), len("hello world"))
	}
}

func TestHandleKey_HomeClearsSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)
	e.HandleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	if !e.selActive {
		t.Fatal("shift+right should activate selection")
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyHome})
	if e.selActive {
		t.Error("plain Home should clear selection")
	}
	if e.cursorCol() != 0 {
		t.Errorf("col after Home = %d, want 0", e.cursorCol())
	}
}
```

- [ ] **Step 2: Run the tests to verify they pass**

All referenced behaviors already exist. No implementation required.

```bash
go test ./... -run "TestHandleKey_DeleteSelection|TestHandleKey_DeleteMouseSelection|TestHandleKey_HomeEndMoveCursor|TestHandleKey_HomeClearsSelection"
```

Expected: PASS.

> **If any test fails**, that's a bug in the existing behavior. Debug the failing case: for Home/End the likely cause is that the bubbles textarea doesn't bind the key on this version, in which case add an explicit `case "home":` / `case "end":` branch to `HandleKey` in `selection.go` that calls `e.ClearSelection()` followed by `e.SetCursor(0)` or `e.CursorEnd()`. For delete-selection failures, inspect the `case "backspace", "delete":` branch in `selection.go:341`.

- [ ] **Step 3: Commit**

```bash
git add selection_test.go
git commit -m "test(selection): cover Home/End, delete-selection, and mouse delete

Adds explicit coverage for line-start/end navigation, Home clearing a
shift-extended selection, and backspace deleting both keyboard- and
mouse-selected text. Confirms mouse and keyboard paths share the same
DeleteSelection code."
```

---

## Task 7: Ctrl+Y manual git sync

**Files:**
- Modify: `git_sync.go`
- Modify: `model.go` (inside the main key switch in `Update`)
- Modify: `git_sync_test.go`

- [ ] **Step 1: Write failing tests for `triggerManualGitSync`**

Append to `git_sync_test.go`:

```go
func TestTriggerManualGitSync_SkipsIfAlreadyRunning(t *testing.T) {
	m := newTestModel(t)
	m.gitSyncRunning = true
	next, cmd := m.triggerManualGitSync()
	if cmd != nil {
		t.Error("expected nil cmd when already running")
	}
	nm := next.(model)
	if !nm.gitSyncRunning {
		t.Error("gitSyncRunning should remain true")
	}
}

func TestTriggerManualGitSync_ConfigErrorSetsErrMsg(t *testing.T) {
	m := newTestModel(t)
	// newTestModel sets XDG_CONFIG_HOME to a tempdir — no config file
	// exists there, so loadConfig fails with "reading config".
	next, cmd := m.triggerManualGitSync()
	if cmd != nil {
		t.Error("expected nil cmd on config error")
	}
	nm := next.(model)
	if nm.errMsg == "" {
		t.Error("errMsg should be set on config error")
	}
}

func TestTriggerManualGitSync_NoRemotePromptsForURL(t *testing.T) {
	m := newTestModel(t)
	xdg := os.Getenv("XDG_CONFIG_HOME")
	cfgDir := filepath.Join(xdg, "clipad")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfgDir, "config.toml"),
		[]byte(`vault = "/tmp/test"`+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}

	next, _ := m.triggerManualGitSync()
	nm := next.(model)
	if nm.inputMode != inputGitRemote {
		t.Errorf("inputMode = %v, want inputGitRemote", nm.inputMode)
	}
}

func TestTriggerManualGitSync_BypassesLastSyncGuard(t *testing.T) {
	m := newTestModel(t)
	remote := initBareRemote(t)
	m.vault = initLocalWithRemote(t, remote)

	xdg := os.Getenv("XDG_CONFIG_HOME")
	cfgDir := filepath.Join(xdg, "clipad")
	os.MkdirAll(cfgDir, 0o755)
	recent := time.Now().Format(time.RFC3339)
	cfg := `vault = "` + m.vault + `"` + "\n" +
		`git_remote = "` + remote + `"` + "\n" +
		`last_sync = "` + recent + `"` + "\n"
	os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(cfg), 0o644)

	next, cmd := m.triggerManualGitSync()
	nm := next.(model)
	if !nm.gitSyncRunning {
		t.Error("gitSyncRunning should be true after manual trigger")
	}
	if cmd == nil {
		t.Error("expected non-nil sync cmd")
	}
}
```

Add to the file's imports: `"time"`.

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./... -run "TestTriggerManualGitSync"
```

Expected: compilation error — `triggerManualGitSync` undefined.

- [ ] **Step 3: Implement `triggerManualGitSync`**

Append to `git_sync.go`:

```go
func (m model) triggerManualGitSync() (tea.Model, tea.Cmd) {
	if m.gitSyncRunning {
		return m, nil
	}
	cfg, err := loadConfig()
	if err != nil {
		m.errMsg = "Git sync: " + err.Error()
		return m, nil
	}
	if cfg.GitRemote == "" {
		m.inputMode = inputGitRemote
		m.gitRemoteInput.SetValue("")
		cmd := m.gitRemoteInput.Focus()
		return m, cmd
	}
	m.gitSyncRunning = true
	m.gitSyncError = ""
	return m, runGitSync(m.vault, cfg.GitRemote)
}
```

- [ ] **Step 4: Wire the Ctrl+Y key in `model.Update`**

In `model.go`, inside the main key switch (around line 366), add a new case alongside the existing global keys (e.g. after `case "ctrl+r":`):

```go
case "ctrl+y":
	return m.triggerManualGitSync()
```

- [ ] **Step 5: Run the tests and verify they pass**

```bash
go test ./... -run "TestTriggerManualGitSync"
```

Expected: PASS.

- [ ] **Step 6: Run the full test suite**

```bash
go test ./...
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add git_sync.go git_sync_test.go model.go
git commit -m "feat(git): Ctrl+Y triggers manual sync

Adds triggerManualGitSync that bypasses the 24-hour guard used by the
periodic sync timer. Honors the existing gitSyncRunning flag, opens the
git-remote prompt when no remote is configured, and surfaces config
errors in the status bar."
```

---

## Task 8: README update and final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add Ctrl+Y to the Global keybindings table**

In `README.md`, in the Global keybindings table (around line 49-57), add a new row for `Ctrl+Y`. The updated section should read:

```markdown
### Global

| Key | Action |
|-----|--------|
| `Ctrl+S` | Save |
| `Ctrl+N` | New note (filename derived from first line) |
| `Ctrl+R` | Find & replace |
| `Ctrl+P` | Toggle markdown preview |
| `Ctrl+Y` | Sync with git remote (push/pull) |
| `Ctrl+Q` | Quit |
| `Tab` | Switch panels |
| `Ctrl+Space` | Open plugin selector |
```

- [ ] **Step 2: Add a new "Mouse" section**

Immediately after the "Editor" keybindings section (right before the "Plugins" heading, around line 77), add:

```markdown
### Mouse

| Action | Effect |
|--------|--------|
| Click in editor | Move cursor to clicked position |
| Click-drag in editor | Select text (same as shift+arrow) |
| Wheel up / down in editor | Scroll editor contents |
| Click on file in tree | Move tree cursor and open file in preview |
| Click on folder in tree | Expand / collapse the folder |
| Wheel up / down in tree | Scroll tree |

Terminal-native selection (dragging with the OS to copy outside the app) is disabled while clipad has the mouse. Most terminals still allow Shift+drag to bypass the app and use the OS selection.
```

- [ ] **Step 3: Build and run the full test suite**

```bash
go build ./... && go test ./...
```

Expected: build success, all tests PASS.

- [ ] **Step 4: Manual smoke test**

Build the binary and walk through the integration checklist. For each item, note PASS/FAIL.

```bash
go build -o clipad . && ./clipad
```

- [ ] Type in editor, shift+arrow select, press backspace → text deleted.
- [ ] Click-drag in editor → text visually highlighted.
- [ ] Click-drag then backspace → text deleted.
- [ ] Click a file in tree → preview opens in right pane; tree stays focused.
- [ ] Click a folder in tree → expand/collapse; cursor moves to that row.
- [ ] Wheel in editor scrolls the view; wheel in tree scrolls the tree.
- [ ] Press `Home` then `End` on a non-empty line → cursor jumps to line start, then line end.
- [ ] Press `Ctrl+Y` → status bar shows "Syncing..." then "Synced" / "Backed up" / a git error.

If any item fails, debug the offending handler (see which task added it) and fix before committing.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document Ctrl+Y manual sync and mouse support"
```

---

## Summary of commits

1. `feat(mouse): add pure coordinate helpers and mouseDragging field`
2. `feat(mouse): add drag-selection and scroll methods on editor`
3. `feat(mouse): route editor clicks, drags, and wheel events`
4. `feat(mouse): route tree clicks and wheel events`
5. `feat(mouse): enable mouse events and wire dispatcher`
6. `test(selection): cover Home/End, delete-selection, and mouse delete`
7. `feat(git): Ctrl+Y triggers manual sync`
8. `docs: document Ctrl+Y manual sync and mouse support`
