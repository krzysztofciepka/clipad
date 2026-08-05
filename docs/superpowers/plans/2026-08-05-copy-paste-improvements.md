# Copy/Paste Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-copy highlighted text to the system clipboard with a green `Copied` confirmation, and make terminal-originated paste an atomic undo step.

**Architecture:** Three small, independent changes to clipad's existing Bubble Tea model. A copy-flash unit (message + tick + priority helper) lives in `clipboard.go` and follows the existing `autoSaveFlash`/`gitSyncFlash` pattern exactly. The mouse handler copies once on drag release. The editor's key handler gains an early branch for bracketed-paste messages so they route through the `editKindOp` undo path instead of the typing path.

**Tech Stack:** Go, Bubble Tea v1.3.10, Bubbles v1.0.0 (textarea), Lipgloss, `github.com/atotto/clipboard`.

**Spec:** `docs/superpowers/specs/2026-08-05-copy-paste-improvements-design.md`

## Global Constraints

- **No git operations.** Do not create branches, stage, commit, push, or tag. The user runs all git commands manually. Each task ends by running tests, not by committing.
- Package is flat `package main` at the repo root. All files live at the repo root; tests are `*_test.go` beside their source.
- Test command is `go test ./...` from `/home/kc/repos/clipad`. A single test runs with `go test -run TestName ./...`.
- Do **not** bind `ctrl+shift+c` or `ctrl+shift+v`. Bubble Tea v1.3.10 has no key name for either (`key.go:311-330` defines only the arrow/home/end `ctrl+shift+*` variants), and terminal emulators consume both keys before they reach the app. This is settled in the spec's Background section — do not attempt a workaround.
- Tests must never write to the real system clipboard. Stub the `writeClipboard` variable introduced in Task 1, following the existing stubbing pattern in `image_copy_test.go:13-24`.
- Flash fade delay is **2 seconds**, matching `autoSaveFadeTick` (`autosave.go:19-23`) and `gitSyncFadeTick`.
- Flash text is exactly `Copied`.

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `clipboard.go` | Modify | Gains the whole copy-flash unit: `writeClipboard` seam, `copyFlashMsg`, `copyFlashTick()`, `flashText()`, and the `model.flashCopied()` helper. Keeps `model.go` (66 KB) from growing. |
| `clipboard_test.go` | Modify | Tests for `flashText` priority and `flashCopied`. |
| `selection.go` | Modify | `copyToClipboard` routes through `writeClipboard`; `Paste()` splits into `PasteText()`; `HandleKey` gains the bracketed-paste branch. |
| `selection_test.go` | Modify | Bracketed-paste undo tests. |
| `model.go` | Modify | `copyFlash` field, `copyFlashMsg` Update case, `View()` flash resolution, `Ctrl+C`/`Ctrl+X` flash wiring. |
| `mouse.go` | Modify | Auto-copy on drag release. |
| `mouse_test.go` | Modify | Drag-release auto-copy tests. |
| `copy_flash_test.go` | Create | `Ctrl+C`/`Ctrl+X` flash tests driven through `model.Update`. |
| `statusbar_test.go` | Modify | `Copied` renders in the status bar's right slot. |
| `README.md` | Modify | Document auto-copy on highlight. |

---

### Task 1: Copy-flash unit

Builds the flash infrastructure and the clipboard test seam. Nothing calls `flashCopied` yet — Tasks 2 and 3 wire it up.

**Files:**
- Modify: `clipboard.go` (append)
- Modify: `selection.go:381-384` (`copyToClipboard`)
- Modify: `model.go:134` (struct field), `model.go:586-588` (Update case), `model.go:2257-2261` (View)
- Test: `clipboard_test.go`, `statusbar_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `var writeClipboard = clipboard.WriteAll` — `func(string) error`, stubbable in tests
  - `type copyFlashMsg struct{}`
  - `func copyFlashTick() tea.Cmd`
  - `func flashText(copyFlash, autoSaveFlash bool, gitSyncFlash string) string`
  - `func (m *model) flashCopied() tea.Cmd`
  - `model.copyFlash bool` field

- [ ] **Step 1: Write the failing tests**

Append to `clipboard_test.go`:

```go
func TestFlashTextPriority(t *testing.T) {
	tests := []struct {
		name          string
		copyFlash     bool
		autoSaveFlash bool
		gitSyncFlash  string
		want          string
	}{
		{"none", false, false, "", ""},
		{"copy only", true, false, "", "Copied"},
		{"autosave only", false, true, "", "Auto-saved"},
		{"gitsync only", false, false, "Synced", "Synced"},
		{"copy beats autosave", true, true, "", "Copied"},
		{"copy beats gitsync", true, false, "Synced", "Copied"},
		{"autosave beats gitsync", false, true, "Backed up", "Auto-saved"},
	}
	for _, tt := range tests {
		got := flashText(tt.copyFlash, tt.autoSaveFlash, tt.gitSyncFlash)
		if got != tt.want {
			t.Errorf("%s: flashText(%v, %v, %q) = %q, want %q",
				tt.name, tt.copyFlash, tt.autoSaveFlash, tt.gitSyncFlash, got, tt.want)
		}
	}
}

func TestFlashCopiedSetsFlagAndReturnsTick(t *testing.T) {
	var m model
	cmd := m.flashCopied()
	if !m.copyFlash {
		t.Error("flashCopied should set copyFlash")
	}
	if cmd == nil {
		t.Fatal("flashCopied should return the fade tick")
	}
	if _, ok := cmd().(copyFlashMsg); !ok {
		t.Errorf("tick produced %T, want copyFlashMsg", cmd())
	}
}

func TestCopyFlashMsgClearsFlag(t *testing.T) {
	m := model{copyFlash: true}
	next, _ := m.Update(copyFlashMsg{})
	if next.(model).copyFlash {
		t.Error("copyFlashMsg should clear copyFlash")
	}
}

func TestWriteClipboardIsStubbable(t *testing.T) {
	var got string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { got = s; return nil }

	e := newSelectableEditor()
	e.copyToClipboard("payload")

	if got != "payload" {
		t.Errorf("writeClipboard received %q, want %q", got, "payload")
	}
	if e.textClip != "payload" {
		t.Errorf("textClip = %q, want %q", e.textClip, "payload")
	}
}
```

Append to `statusbar_test.go`:

```go
func TestViewShowsCopiedFlash(t *testing.T) {
	sb := StatusBar{width: 120, filename: "note.md", flashMsg: "Copied"}
	out := sb.View()
	if !strings.Contains(out, "Copied") {
		t.Errorf("View() = %q, want it to contain %q", out, "Copied")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestFlashText|TestFlashCopied|TestCopyFlashMsg|TestWriteClipboardIsStubbable|TestViewShowsCopiedFlash' ./...`

Expected: build failure — `undefined: flashText`, `undefined: writeClipboard`, `m.flashCopied undefined`, `undefined: copyFlashMsg`, `m.copyFlash undefined`.

`TestViewShowsCopiedFlash` will pass once the package compiles (`StatusBar.flashMsg` already renders), but it cannot run until the rest compiles. That is expected — it is a guard on the existing render path.

- [ ] **Step 3: Add the copy-flash unit to `clipboard.go`**

Extend the import block and append to the end of the file:

```go
import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)
```

```go
// writeClipboard writes text to the system clipboard. Indirected through a
// variable so tests can observe copies without clobbering the real clipboard.
var writeClipboard = clipboard.WriteAll

// copyFlashMsg clears the "Copied" status-bar confirmation.
type copyFlashMsg struct{}

// copyFlashTick fades the copy confirmation after the same delay as the
// auto-save and git-sync flashes.
func copyFlashTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return copyFlashMsg{}
	})
}

// flashCopied shows the green "Copied" confirmation in the status bar and
// returns the command that clears it.
func (m *model) flashCopied() tea.Cmd {
	m.copyFlash = true
	return copyFlashTick()
}

// flashText resolves which non-error flash the status bar shows. The copy
// confirmation wins: it acknowledges an explicit user action, where auto-save
// and git-sync are background events the user did not just trigger.
func flashText(copyFlash, autoSaveFlash bool, gitSyncFlash string) string {
	switch {
	case copyFlash:
		return "Copied"
	case autoSaveFlash:
		return "Auto-saved"
	default:
		return gitSyncFlash
	}
}
```

- [ ] **Step 4: Route `copyToClipboard` through the seam**

In `selection.go:381-384`, change:

```go
func (e *SelectableEditor) copyToClipboard(text string) {
	e.textClip = text
	clipboard.WriteAll(text)
}
```

to:

```go
func (e *SelectableEditor) copyToClipboard(text string) {
	e.textClip = text
	writeClipboard(text)
}
```

Leave the `github.com/atotto/clipboard` import in `selection.go` — `readFromClipboard` still uses `clipboard.ReadAll`.

- [ ] **Step 5: Add the `copyFlash` field**

In `model.go`, in the `model` struct, next to `autoSaveFlash`:

```go
	fileClip      fileClipboard
	autoSaveFlash bool
	copyFlash     bool
```

- [ ] **Step 6: Handle `copyFlashMsg` in Update**

In `model.go`, immediately after the `case autoSaveFadeMsg:` block (around line 586-588):

```go
	case autoSaveFadeMsg:
		m.autoSaveFlash = false
		return m, nil

	case copyFlashMsg:
		m.copyFlash = false
		return m, nil
```

- [ ] **Step 7: Use `flashText` in View**

In `model.go` (around line 2257), replace:

```go
	if m.autoSaveFlash {
		sb.flashMsg = "Auto-saved"
	} else if m.gitSyncFlash != "" {
		sb.flashMsg = m.gitSyncFlash
	}
```

with:

```go
	sb.flashMsg = flashText(m.copyFlash, m.autoSaveFlash, m.gitSyncFlash)
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test -run 'TestFlashText|TestFlashCopied|TestCopyFlashMsg|TestWriteClipboardIsStubbable|TestViewShowsCopiedFlash' ./...`

Expected: PASS.

- [ ] **Step 9: Run the full suite**

Run: `go test ./...`

Expected: PASS, no regressions. Do not commit.

---

### Task 2: Flash on explicit copy and cut

Wires `Ctrl+C` and `Ctrl+X` in the editor to the flash. Both are silent today — only the tree panel and the image path give feedback.

**Files:**
- Modify: `model.go:752-770` (`ctrl+c`), `model.go:772-790` (`ctrl+x`)
- Test: Create `copy_flash_test.go`

**Interfaces:**
- Consumes: `writeClipboard`, `model.copyFlash`, `(*model).flashCopied()` from Task 1.
- Produces: no new exported names.

- [ ] **Step 1: Write the failing tests**

Create `copy_flash_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// selectHello puts "hello world" in the editor with "hello" selected and the
// editor panel focused, ready for a copy or cut key.
func selectHello(t *testing.T) model {
	t.Helper()
	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	m.editor.StartMouseDrag(0, 0)
	m.editor.UpdateMouseDrag(0, 5)
	m.editor.EndMouseDrag()
	if m.editor.SelectedText() != "hello" {
		t.Fatalf("setup: SelectedText = %q, want %q", m.editor.SelectedText(), "hello")
	}
	return m
}

func TestCtrlCCopiesAndFlashes(t *testing.T) {
	var wrote string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { wrote = s; return nil }

	m := selectHello(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(model)

	if wrote != "hello" {
		t.Errorf("clipboard = %q, want %q", wrote, "hello")
	}
	if !nm.copyFlash {
		t.Error("copyFlash should be set after ctrl+c")
	}
	if cmd == nil {
		t.Error("ctrl+c should return the flash fade tick")
	}
}

func TestCtrlXCutsAndFlashes(t *testing.T) {
	var wrote string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { wrote = s; return nil }

	m := selectHello(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	nm := next.(model)

	if wrote != "hello" {
		t.Errorf("clipboard = %q, want %q", wrote, "hello")
	}
	if nm.editor.Value() != " world" {
		t.Errorf("buffer after cut = %q, want %q", nm.editor.Value(), " world")
	}
	if !nm.copyFlash {
		t.Error("copyFlash should be set after ctrl+x")
	}
	if cmd == nil {
		t.Error("ctrl+x should return the flash fade tick")
	}
}

func TestCtrlCWithoutSelectionDoesNotFlash(t *testing.T) {
	var called bool
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(string) error { called = true; return nil }

	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")
	m.activePanel = editorPanel
	m.editorMode = modeEdit

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(model)

	if called {
		t.Error("ctrl+c without a selection should not write the clipboard")
	}
	if nm.copyFlash {
		t.Error("ctrl+c without a selection should not flash")
	}
	if cmd != nil {
		t.Error("ctrl+c without a selection should return no command")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestCtrlC|TestCtrlX' ./...`

Expected: `TestCtrlCCopiesAndFlashes` and `TestCtrlXCutsAndFlashes` FAIL with "copyFlash should be set" and "should return the flash fade tick". `TestCtrlCWithoutSelectionDoesNotFlash` already passes — it is the guard that the new code does not over-fire.

- [ ] **Step 3: Wire the flash into `ctrl+c`**

In `model.go`, in the `case "ctrl+c":` branch, replace the editor block:

```go
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				if target, ok := m.editor.LoneImageElement(); ok {
					if m.copyImageElement(target, false) {
						return m, nil
					}
				}
				m.editor.Copy()
			}
			return m, nil
```

with:

```go
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				if target, ok := m.editor.LoneImageElement(); ok {
					if m.copyImageElement(target, false) {
						return m, nil
					}
				}
				// SelectedText returns "" when no selection is active, so this
				// covers both "nothing selected" and "selection spans nothing".
				if m.editor.SelectedText() == "" {
					return m, nil
				}
				m.editor.Copy()
				return m, m.flashCopied()
			}
			return m, nil
```

- [ ] **Step 4: Wire the flash into `ctrl+x`**

In `model.go`, in the `case "ctrl+x":` branch, replace the editor block:

```go
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				if target, ok := m.editor.LoneImageElement(); ok {
					if m.copyImageElement(target, true) {
						return m, nil
					}
				}
				m.editor.Cut()
			}
			return m, nil
```

with:

```go
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				if target, ok := m.editor.LoneImageElement(); ok {
					if m.copyImageElement(target, true) {
						return m, nil
					}
				}
				// Checked before Cut, which clears the selection as it works.
				if m.editor.SelectedText() == "" {
					return m, nil
				}
				m.editor.Cut()
				return m, m.flashCopied()
			}
			return m, nil
```

Leave the tree-panel branches of both handlers untouched — they keep their `Copied: <name>` / `Cut: <name>` messages.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -run 'TestCtrlC|TestCtrlX' ./...`

Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`

Expected: PASS, no regressions. Do not commit.

---

### Task 3: Auto-copy on mouse-drag release

The headline feature. One clipboard write per drag, on release only — nothing happens during motion.

**Files:**
- Modify: `mouse.go:220-224` (`MouseActionRelease` in `handleEditorMouse`)
- Test: `mouse_test.go` (append)
- Modify: `README.md`

**Interfaces:**
- Consumes: `writeClipboard`, `model.copyFlash`, `(*model).flashCopied()` from Task 1.
- Produces: no new names. `handleEditorMouse` keeps its `(m model, localX, localY int, msg tea.MouseMsg) (tea.Model, tea.Cmd)` signature.

- [ ] **Step 1: Write the failing tests**

Append to `mouse_test.go`:

```go
func TestHandleEditorMouse_DragReleaseAutoCopies(t *testing.T) {
	var wrote string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { wrote = s; return nil }

	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, press)
	m = next.(model)

	motion := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion}
	next, _ = handleEditorMouse(m, 9, 0, motion)
	m = next.(model)

	if wrote != "" {
		t.Errorf("clipboard written during motion (%q); copy must wait for release", wrote)
	}

	release := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	next, cmd := handleEditorMouse(m, 9, 0, release)
	m = next.(model)

	if wrote != "ello" {
		t.Errorf("clipboard = %q, want %q", wrote, "ello")
	}
	if !m.copyFlash {
		t.Error("copyFlash should be set after auto-copy")
	}
	if cmd == nil {
		t.Error("release with a selection should return the flash fade tick")
	}
	if !m.editor.selActive {
		t.Error("selection should survive the auto-copy")
	}
}

func TestHandleEditorMouse_ClickWithoutDragDoesNotCopy(t *testing.T) {
	var called bool
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(string) error { called = true; return nil }

	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, press)
	m = next.(model)

	release := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	next, cmd := handleEditorMouse(m, 5, 0, release)
	m = next.(model)

	if called {
		t.Error("a plain click should not write the clipboard")
	}
	if m.copyFlash {
		t.Error("a plain click should not flash Copied")
	}
	if cmd != nil {
		t.Error("a plain click should return no command")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestHandleEditorMouse_ ./...`

Expected: `TestHandleEditorMouse_DragReleaseAutoCopies` FAILS with `clipboard = "", want "ello"`. `TestHandleEditorMouse_ClickWithoutDragDoesNotCopy` already passes — it guards against over-firing.

- [ ] **Step 3: Auto-copy on release**

In `mouse.go`, in `handleEditorMouse`'s `case tea.MouseButtonLeft:` switch, replace:

```go
		case tea.MouseActionRelease:
			m.editor.EndMouseDrag()
		}
		return m, nil
```

with:

```go
		case tea.MouseActionRelease:
			m.editor.EndMouseDrag()
			// Highlighting with the mouse copies straight to the system
			// clipboard. EndMouseDrag already cleared the selection for a
			// click without a drag, so a plain click cannot reach this.
			if m.editor.SelectedText() != "" {
				m.editor.Copy()
				return m, m.flashCopied()
			}
		}
		return m, nil
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -run TestHandleEditorMouse_ ./...`

Expected: PASS.

- [ ] **Step 5: Document it in the README**

In `README.md`, in the `## Features` list, add a bullet after the **Markdown editor** entry:

```markdown
- **Auto-copy on highlight** — selecting text with the mouse copies it to the system clipboard, confirmed by a green `Copied` flash in the status bar
```

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`

Expected: PASS, no regressions. Do not commit.

---

### Task 4: Bracketed paste as an atomic undo step

`Ctrl+Shift+V` reaches clipad as a bracketed paste (`KeyMsg{Type: KeyRunes, Paste: true}` carrying every rune, per `key_sequences.go:83-119`). Today it lands in the generic typing path and coalesces into the surrounding `editKindTyping` undo group, so one `Ctrl+Z` after a paste also undoes the typing around it.

**Files:**
- Modify: `selection.go:415-431` (`Paste`), `selection.go:525-529` (`HandleKey` head)
- Test: `selection_test.go` (append)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func (e *SelectableEditor) PasteText(text string)` — inserts at the cursor as one undo operation, replacing any active selection first. `Paste() tea.Cmd` keeps its signature and always-nil return.

- [ ] **Step 1: Write the failing tests**

Append to `selection_test.go`:

```go
func TestPasteTextInsertsAtCursor(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("ac")
	setEditorSize(&e, 80, 10)
	e.moveTo(0, 1)

	e.PasteText("b")

	if e.Value() != "abc" {
		t.Errorf("Value = %q, want %q", e.Value(), "abc")
	}
}

func TestBracketedPasteIsItsOwnUndoStep(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("")
	setEditorSize(&e, 80, 10)

	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if e.Value() != "ab" {
		t.Fatalf("after typing = %q, want %q", e.Value(), "ab")
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("XY"), Paste: true})
	if e.Value() != "abXY" {
		t.Fatalf("after paste = %q, want %q", e.Value(), "abXY")
	}

	e.Undo()
	if e.Value() != "ab" {
		t.Errorf("after undo = %q, want %q — a terminal paste must undo "+
			"on its own, not together with the typing before it", e.Value(), "ab")
	}
}

func TestBracketedPasteWithNewlines(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("")
	setEditorSize(&e, 80, 10)

	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo"), Paste: true})

	if e.Value() != "one\ntwo" {
		t.Errorf("Value = %q, want %q", e.Value(), "one\ntwo")
	}
}

func TestBracketedPasteReplacesSelectionInOneUndo(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)
	e.StartMouseDrag(0, 0)
	e.UpdateMouseDrag(0, 5)
	e.EndMouseDrag()

	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("bye"), Paste: true})
	if e.Value() != "bye world" {
		t.Fatalf("after paste = %q, want %q", e.Value(), "bye world")
	}
	if e.selActive {
		t.Error("selection should be cleared after a paste replaces it")
	}

	e.Undo()
	if e.Value() != "hello world" {
		t.Errorf("after undo = %q, want %q", e.Value(), "hello world")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run 'TestPasteText|TestBracketedPaste' ./...`

Expected: build failure — `e.PasteText undefined`. After Step 3 defines it but before Step 4, `TestBracketedPasteIsItsOwnUndoStep` FAILS with `after undo = "", want "ab"` (the paste currently merges into the typing group). `TestBracketedPasteReplacesSelectionInOneUndo` passes throughout — it locks in that the new path preserves the existing replace-selection behavior.

- [ ] **Step 3: Split `Paste` into `PasteText`**

In `selection.go`, replace:

```go
func (e *SelectableEditor) Paste() tea.Cmd {
	text := e.readFromClipboard()
	if text == "" {
		return nil
	}
	pre := e.recordOp()
	if e.selActive {
		sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
		newContent := deleteText(e.Value(), sL, sC, eL, eC)
		e.SetValue(newContent)
		e.moveTo(sL, sC)
		e.selActive = false
	}
	e.InsertString(text)
	e.commitOp(pre)
	return nil
}
```

with:

```go
// PasteText inserts text at the cursor as a single undo operation, replacing
// any active selection first. recordOp uses editKindOp, which always starts a
// new undo group, so a paste never merges into surrounding typing.
func (e *SelectableEditor) PasteText(text string) {
	if text == "" {
		return
	}
	pre := e.recordOp()
	if e.selActive {
		sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
		newContent := deleteText(e.Value(), sL, sC, eL, eC)
		e.SetValue(newContent)
		e.moveTo(sL, sC)
		e.selActive = false
	}
	e.InsertString(text)
	e.commitOp(pre)
}

func (e *SelectableEditor) Paste() tea.Cmd {
	e.PasteText(e.readFromClipboard())
	return nil
}
```

- [ ] **Step 4: Route bracketed paste through `PasteText`**

In `selection.go`, at the head of `HandleKey`, insert the branch before the `switch key {`:

```go
func (e *SelectableEditor) HandleKey(msg tea.KeyMsg) tea.Cmd {
	defer e.syncVisualYOffset()
	key := msg.String()

	// A terminal paste (Ctrl+Shift+V) arrives as one KeyRunes message
	// carrying every rune, newlines included. Insert it as a single undo
	// operation instead of letting the typing path below merge it into the
	// surrounding edit group.
	if msg.Type == tea.KeyRunes && msg.Paste {
		e.PasteText(string(msg.Runes))
		return nil
	}

	switch key {
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -run 'TestPasteText|TestBracketedPaste' ./...`

Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`

Expected: PASS, no regressions. Do not commit.

---

### Task 5: Build and manual verification

**Files:** none modified.

**Interfaces:**
- Consumes: everything from Tasks 1-4.
- Produces: nothing.

- [ ] **Step 1: Vet and build**

Run:
```bash
go vet ./...
go build -o /tmp/clipad-p12 .
```

Expected: both succeed with no output.

- [ ] **Step 2: Run the full suite one final time**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Report manual-verification steps to the user**

The following need a real terminal and are for the user to confirm — do not attempt to automate them:

1. Highlight text in the editor with the mouse → the green `Copied` flash appears bottom-right, and the text pastes into another application.
2. Click without dragging → no flash.
3. Select with the mouse, press `Ctrl+C` → `Copied` flash.
4. Select with the mouse, press `Ctrl+X` → text is cut and the `Copied` flash appears.
5. Press `Ctrl+Shift+V` to paste multi-line text from the terminal, then `Ctrl+Z` → only the pasted text is removed, typing before it survives.
6. `Ctrl+V` still pastes clipboard images as inline image elements.

---

## Notes for the implementer

- **`SelectedText()` is the emptiness check throughout.** It returns `""` when `selActive` is false (`selection.go:222-228`), so a single `!= ""` test covers both "no selection" and "selection spans nothing". Do not add a separate `selActive` check.
- **`EndMouseDrag` already handles the click case.** `selection.go:298-303` clears the selection when the cursor never left the anchor, so a plain click cannot trigger auto-copy.
- **`m` is addressable in every call site.** `flashCopied` has a pointer receiver; `handleEditorMouse` takes `m model` by value and `Update` has a value receiver, but both are local variables, so `m.flashCopied()` compiles without an explicit `&m`.
- **A bracketed paste's `msg.String()` is the pasted text wrapped in `[ ]`** (`key.go:73-85`), so it matches no case in the model's key switch and falls through to `m.editor.HandleKey` — via `handleEditorKeys` (`model.go:1137`) or the tree panel's printable-input auto-switch (`model.go:1084`). No model-level routing change is needed.
- **Clipboard write errors stay swallowed.** `copyToClipboard` ignores the error and keeps the in-process `textClip` fallback that clipad's own `Ctrl+V` reads. This plan does not change that.
