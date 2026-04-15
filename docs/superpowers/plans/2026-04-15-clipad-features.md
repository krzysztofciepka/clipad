# Clipad Feature Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add move/copy files, auto-save, text selection, and AI shortcuts to clipad.

**Architecture:** Four independent features built as separate files, integrated into the Bubble Tea model. Text selection wraps the existing textarea in a SelectableEditor with selection tracking. AI shortcuts reuse the OpenRouter HTTP backend via an extracted helper function.

**Tech Stack:** Go 1.26, Bubble Tea (bubbletea v1.3.10), Lipgloss, TOML (pelletier/go-toml/v2), atotto/clipboard (already indirect dep)

---

## File Structure

**New files:**

| File | Responsibility |
|------|----------------|
| `autosave.go` | Tick message types, tick commands, auto-save handler |
| `clipboard.go` | File clipboard struct, copyFile helper, uniquePath helper |
| `clipboard_test.go` | Tests for copyFile, uniquePath |
| `selection.go` | SelectableEditor wrapping textarea, selection tracking, word boundaries, custom rendering, text clipboard |
| `selection_test.go` | Tests for word boundaries, selection range, selected text, delete selection |
| `shortcuts.go` | AIShortcut struct, TOML load/save, LLM execution via callOpenRouter |
| `shortcuts_test.go` | Tests for shortcut load/save |
| `shortcuts_input.go` | Input handlers for shortcut menu, name/prompt forms, delete confirm |
| `shortcuts_modal.go` | Shortcut context menu rendering |

**Modified files:**

| File | Changes |
|------|---------|
| `model.go` | New fields, new input modes, message handlers, key handler rewiring |
| `editor.go` | newSelectableEditor(), update setEditorSize signature |
| `statusbar.go` | Add flashMsg field for non-error status messages |
| `plugin_openrouter.go` | Extract callOpenRouter() shared HTTP helper |
| `tree.go` | Visual dimming for cut files |

---

### Task 1: Auto-save

**Files:**
- Create: `autosave.go`
- Modify: `statusbar.go`
- Modify: `model.go`

- [ ] **Step 1: Create autosave.go**

```go
// autosave.go
package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type autoSaveTickMsg struct{}
type autoSaveFadeMsg struct{}

func autoSaveTick() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg {
		return autoSaveTickMsg{}
	})
}

func autoSaveFadeTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return autoSaveFadeMsg{}
	})
}
```

- [ ] **Step 2: Add flashMsg to StatusBar in statusbar.go**

Add `flashMsg string` field to StatusBar struct and render it in green when set (takes priority over filename but not errMsg):

In `statusbar.go`, add to the struct:
```go
type StatusBar struct {
	width      int
	treeActive bool
	filename   string
	line       int
	col        int
	dirty      bool
	errMsg     string
	flashMsg   string // non-error flash message (e.g. "Auto-saved")
	fileOpen   bool
}
```

In `StatusBar.View()`, change the right-side rendering block:
```go
	// Replace existing right-side block with:
	right := ""
	if s.errMsg != "" {
		right = statusErrorStyle.Render(s.errMsg)
	} else if s.flashMsg != "" {
		right = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("76")).
			Render(s.flashMsg)
	} else if s.filename != "" {
		modified := ""
		if s.dirty {
			modified = " [+]"
		}
		right = fmt.Sprintf("%d:%d  %s%s", s.line, s.col, s.filename, modified)
	}
```

- [ ] **Step 3: Add auto-save handling to model.go**

Add field to model struct:
```go
	autoSaveFlash bool
```

In `Init()`, add autoSaveTick to the batch:
```go
func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, watchVault(m.vault), autoSaveTick())
}
```

In `Update()`, add message handlers before the `tea.KeyMsg` case:
```go
	case autoSaveTickMsg:
		if m.currentFile != "" && m.isDirty() {
			m.saveCurrentFile()
			if m.errMsg == "" {
				m.autoSaveFlash = true
				return m, tea.Batch(autoSaveTick(), autoSaveFadeTick())
			}
		}
		return m, autoSaveTick()

	case autoSaveFadeMsg:
		m.autoSaveFlash = false
		return m, nil
```

In `View()`, set flashMsg when building the StatusBar:
```go
	sb := StatusBar{
		width:      m.width,
		treeActive: m.activePanel == treePanel,
		filename:   filename,
		line:       line + 1,
		col:        col + 1,
		dirty:      m.isDirty(),
		errMsg:     m.errMsg,
		fileOpen:   m.currentFile != "" || m.newNoteDir != "",
	}
	if m.autoSaveFlash {
		sb.flashMsg = "Auto-saved"
	}
```

- [ ] **Step 4: Run build and tests**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add autosave.go statusbar.go model.go
git commit -m "feat: add auto-save with 15s interval and status bar flash"
```

---

### Task 2: File clipboard (cut/copy/paste in tree)

**Files:**
- Create: `clipboard.go`
- Create: `clipboard_test.go`
- Modify: `model.go`
- Modify: `tree.go`

- [ ] **Step 1: Write tests for copyFile and uniquePath**

```go
// clipboard_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.md")
	dst := filepath.Join(dir, "dest.md")
	os.WriteFile(src, []byte("hello"), 0o644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("dest content = %q, want %q", string(data), "hello")
	}
}

func TestCopyFile_SrcMissing(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nope.md"), filepath.Join(dir, "dst.md"))
	if err == nil {
		t.Error("expected error for missing source, got nil")
	}
}

func TestUniquePath_NoConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	got := uniquePath(path)
	if got != path {
		t.Errorf("uniquePath() = %q, want %q", got, path)
	}
}

func TestUniquePath_Conflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	os.WriteFile(path, []byte(""), 0o644)

	got := uniquePath(path)
	want := filepath.Join(dir, "note (1).md")
	if got != want {
		t.Errorf("uniquePath() = %q, want %q", got, want)
	}
}

func TestUniquePath_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	os.WriteFile(path, []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "note (1).md"), []byte(""), 0o644)

	got := uniquePath(path)
	want := filepath.Join(dir, "note (2).md")
	if got != want {
		t.Errorf("uniquePath() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run TestCopyFile -v
go test ./... -run TestUniquePath -v
```

Expected: FAIL — `copyFile` and `uniquePath` not defined.

- [ ] **Step 3: Implement clipboard.go**

```go
// clipboard.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type clipOp int

const (
	clipCut clipOp = iota
	clipCopy
)

type fileClipboard struct {
	path string
	op   clipOp
}

func (c fileClipboard) empty() bool {
	return c.path == ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestCopyFile|TestUniquePath" -v
```

Expected: all PASS.

- [ ] **Step 5: Remove ctrl+c from quit bindings in model.go**

In `model.go`, change every occurrence of `"ctrl+q", "ctrl+c"` to just `"ctrl+q"`. This appears in:
- `Update()` global handler (line ~212)
- `handlePluginSelect()` (line ~39 in plugin_input.go)
- `handlePluginConfig()` (line ~76 in plugin_input.go)
- `handlePluginPrompt()` (line ~113 in plugin_input.go)
- `handlePluginDiff()` (line ~43 in plugin_diff.go)
- `handleNewFolder()` (line ~529 in model.go)
- `handleReplaceSearch()` (line ~568 in model.go)
- `handleReplaceWith()` (similar in model.go)

Search for `"ctrl+q", "ctrl+c"` and replace with `"ctrl+q"` in all files.

- [ ] **Step 6: Add file clipboard to model and tree key handlers**

Add field to model struct in `model.go`:
```go
	fileClip fileClipboard
```

In `handleTreeKeys()`, add these cases before the `default:` case:
```go
	case "ctrl+x":
		node := m.tree.selectedNode()
		if node != nil && !node.IsDir {
			m.fileClip = fileClipboard{path: node.Path, op: clipCut}
			m.errMsg = "Cut: " + node.Name
		}
	case "ctrl+c":
		node := m.tree.selectedNode()
		if node != nil && !node.IsDir {
			m.fileClip = fileClipboard{path: node.Path, op: clipCopy}
			m.errMsg = "Copied: " + node.Name
		}
	case "ctrl+v":
		if !m.fileClip.empty() {
			m.pasteFile()
		}
```

Add the pasteFile method to model.go:
```go
func (m *model) pasteFile() {
	src := m.fileClip.path
	if _, err := os.Stat(src); err != nil {
		m.errMsg = "Source file not found"
		m.fileClip = fileClipboard{}
		return
	}

	// Determine target directory
	dir := m.vault
	node := m.tree.selectedNode()
	if node != nil {
		if node.IsDir {
			dir = node.Path
		} else {
			dir = filepath.Dir(node.Path)
		}
	}

	dst := uniquePath(filepath.Join(dir, filepath.Base(src)))

	if m.fileClip.op == clipCut {
		if err := os.Rename(src, dst); err != nil {
			m.errMsg = fmt.Sprintf("Move failed: %v", err)
			return
		}
		// Update currentFile if the moved file was open
		if m.currentFile == src {
			m.currentFile = dst
			m.tree.currentFile = dst
		}
	} else {
		if err := copyFile(src, dst); err != nil {
			m.errMsg = fmt.Sprintf("Copy failed: %v", err)
			return
		}
	}

	m.fileClip = fileClipboard{}
	m.errMsg = ""
	m.refreshTree()
}
```

- [ ] **Step 7: Add visual dimming for cut files in tree.go**

In `tree.go`, add a `cutPath` field to TreePanel:
```go
type TreePanel struct {
	root        *TreeNode
	items       []FlatItem
	cursor      int
	offset      int
	height      int
	width       int
	currentFile string
	cutPath     string // path of file marked for cut (dimmed display)
}
```

In `TreePanel.View()`, add dimming for cut files. In the file rendering block (where `treeFileStyle` and `treeActiveFile` are used), add a check:
```go
			} else {
				icon = "  "
				if item.Node.Path == tp.currentFile {
					name = treeActiveFile.Render(item.Node.Name)
				} else if item.Node.Path == tp.cutPath {
					name = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true).Render(item.Node.Name)
				} else {
					name = treeFileStyle.Render(item.Node.Name)
				}
			}
```

In `model.go`, sync the cutPath. In handleTreeKeys `ctrl+x` case, add:
```go
		m.tree.cutPath = node.Path
```

In handleTreeKeys `ctrl+c` case and in `pasteFile()` after clearing fileClip:
```go
		m.tree.cutPath = ""
```

- [ ] **Step 8: Run build and tests**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.

- [ ] **Step 9: Commit**

```bash
git add clipboard.go clipboard_test.go model.go tree.go plugin_input.go plugin_diff.go
git commit -m "feat: add file cut/copy/paste in tree panel (ctrl+x/c/v)"
```

---

### Task 3: SelectableEditor — core struct, word boundaries, helpers

**Files:**
- Create: `selection.go`
- Create: `selection_test.go`

- [ ] **Step 1: Write tests for word boundaries and selection helpers**

```go
// selection_test.go
package main

import (
	"testing"
)

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true}, {'Z', true}, {'0', true}, {'_', true},
		{' ', false}, {'.', false}, {'-', false}, {'\n', false},
	}
	for _, tt := range tests {
		if got := isWordChar(tt.r); got != tt.want {
			t.Errorf("isWordChar(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestWordLeftPos(t *testing.T) {
	tests := []struct {
		content  string
		line     int
		col      int
		wantLine int
		wantCol  int
	}{
		{"hello world", 0, 11, 0, 6},    // end of "world" → start of "world"
		{"hello world", 0, 6, 0, 0},     // start of "world" → start of "hello"
		{"hello world", 0, 8, 0, 6},     // middle of "world" → start of "world"
		{"hello world", 0, 0, 0, 0},     // already at start
		{"first\nsecond", 1, 0, 0, 0},   // start of line 2 → start of "first"
		{"hello  world", 0, 7, 0, 0},    // after double space → start of "hello"
	}
	for _, tt := range tests {
		gotLine, gotCol := wordLeftPos(tt.content, tt.line, tt.col)
		if gotLine != tt.wantLine || gotCol != tt.wantCol {
			t.Errorf("wordLeftPos(%q, %d, %d) = (%d, %d), want (%d, %d)",
				tt.content, tt.line, tt.col, gotLine, gotCol, tt.wantLine, tt.wantCol)
		}
	}
}

func TestWordRightPos(t *testing.T) {
	tests := []struct {
		content  string
		line     int
		col      int
		wantLine int
		wantCol  int
	}{
		{"hello world", 0, 0, 0, 6},     // start of "hello" → start of "world"
		{"hello world", 0, 6, 0, 11},    // start of "world" → end of line
		{"hello world", 0, 3, 0, 6},     // middle of "hello" → start of "world"
		{"first\nsecond", 0, 0, 0, 5},   // start → end of "first"
		{"first\nsecond", 0, 5, 1, 0},   // end of "first" → start of "second"
	}
	for _, tt := range tests {
		gotLine, gotCol := wordRightPos(tt.content, tt.line, tt.col)
		if gotLine != tt.wantLine || gotCol != tt.wantCol {
			t.Errorf("wordRightPos(%q, %d, %d) = (%d, %d), want (%d, %d)",
				tt.content, tt.line, tt.col, gotLine, gotCol, tt.wantLine, tt.wantCol)
		}
	}
}

func TestSelectionRange(t *testing.T) {
	// Anchor before cursor
	sL, sC, eL, eC := selectionRange(0, 0, 1, 5)
	if sL != 0 || sC != 0 || eL != 1 || eC != 5 {
		t.Errorf("selectionRange(0,0,1,5) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}

	// Anchor after cursor (reversed)
	sL, sC, eL, eC = selectionRange(1, 5, 0, 0)
	if sL != 0 || sC != 0 || eL != 1 || eC != 5 {
		t.Errorf("selectionRange(1,5,0,0) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}

	// Same line
	sL, sC, eL, eC = selectionRange(2, 10, 2, 3)
	if sL != 2 || sC != 3 || eL != 2 || eC != 10 {
		t.Errorf("selectionRange(2,10,2,3) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
}

func TestExtractText(t *testing.T) {
	content := "hello world\nfoo bar\nbaz"

	tests := []struct {
		sLine, sCol, eLine, eCol int
		want                     string
	}{
		{0, 0, 0, 5, "hello"},
		{0, 6, 0, 11, "world"},
		{0, 0, 1, 3, "hello world\nfoo"},
		{0, 0, 2, 3, "hello world\nfoo bar\nbaz"},
		{1, 4, 2, 3, "bar\nbaz"},
	}
	for _, tt := range tests {
		got := extractText(content, tt.sLine, tt.sCol, tt.eLine, tt.eCol)
		if got != tt.want {
			t.Errorf("extractText(%d,%d,%d,%d) = %q, want %q",
				tt.sLine, tt.sCol, tt.eLine, tt.eCol, got, tt.want)
		}
	}
}

func TestDeleteText(t *testing.T) {
	content := "hello world\nfoo bar\nbaz"

	tests := []struct {
		sLine, sCol, eLine, eCol int
		want                     string
	}{
		{0, 5, 0, 11, "hello\nfoo bar\nbaz"},
		{0, 0, 0, 5, " world\nfoo bar\nbaz"},
		{0, 5, 1, 4, "hellobar\nbaz"},
		{0, 0, 2, 3, ""},
	}
	for _, tt := range tests {
		got := deleteText(content, tt.sLine, tt.sCol, tt.eLine, tt.eCol)
		if got != tt.want {
			t.Errorf("deleteText(%d,%d,%d,%d) = %q, want %q",
				tt.sLine, tt.sCol, tt.eLine, tt.eCol, got, tt.want)
		}
	}
}

func TestPosInRange(t *testing.T) {
	// Selection from (0,2) to (1,3)
	tests := []struct {
		line, col int
		want      bool
	}{
		{0, 0, false}, {0, 1, false}, {0, 2, true}, {0, 5, true},
		{1, 0, true}, {1, 2, true}, {1, 3, false}, {1, 4, false},
		{2, 0, false},
	}
	for _, tt := range tests {
		got := posInRange(tt.line, tt.col, 0, 2, 1, 3)
		if got != tt.want {
			t.Errorf("posInRange(%d, %d, 0,2,1,3) = %v, want %v", tt.line, tt.col, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestIsWordChar|TestWordLeftPos|TestWordRightPos|TestSelectionRange|TestExtractText|TestDeleteText|TestPosInRange" -v
```

Expected: FAIL — functions not defined.

- [ ] **Step 3: Implement selection.go — struct and helper functions**

```go
// selection.go
package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var selectionStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

type SelectableEditor struct {
	textarea.Model
	height        int // stored for custom rendering viewport
	selActive     bool
	selAnchorLine int
	selAnchorCol  int
	textClip      string
	viewOffset    int
}

// --- Pure helper functions (no receiver, easy to test) ---

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func selectionRange(anchorLine, anchorCol, curLine, curCol int) (sLine, sCol, eLine, eCol int) {
	if anchorLine < curLine || (anchorLine == curLine && anchorCol < curCol) {
		return anchorLine, anchorCol, curLine, curCol
	}
	return curLine, curCol, anchorLine, anchorCol
}

func posInRange(line, col, sLine, sCol, eLine, eCol int) bool {
	if line < sLine || line > eLine {
		return false
	}
	if line == sLine && line == eLine {
		return col >= sCol && col < eCol
	}
	if line == sLine {
		return col >= sCol
	}
	if line == eLine {
		return col < eCol
	}
	return true
}

func extractText(content string, sLine, sCol, eLine, eCol int) string {
	lines := strings.Split(content, "\n")
	if sLine == eLine {
		runes := []rune(lines[sLine])
		if eCol > len(runes) {
			eCol = len(runes)
		}
		return string(runes[sCol:eCol])
	}
	var b strings.Builder
	// First line from sCol to end
	runes := []rune(lines[sLine])
	b.WriteString(string(runes[sCol:]))
	// Middle lines (full)
	for i := sLine + 1; i < eLine; i++ {
		b.WriteByte('\n')
		b.WriteString(lines[i])
	}
	// Last line from start to eCol
	b.WriteByte('\n')
	runes = []rune(lines[eLine])
	if eCol > len(runes) {
		eCol = len(runes)
	}
	b.WriteString(string(runes[:eCol]))
	return b.String()
}

func deleteText(content string, sLine, sCol, eLine, eCol int) string {
	lines := strings.Split(content, "\n")
	startRunes := []rune(lines[sLine])
	endRunes := []rune(lines[eLine])
	if eCol > len(endRunes) {
		eCol = len(endRunes)
	}

	// Build new content: before selection + after selection
	before := string(startRunes[:sCol])
	after := string(endRunes[eCol:])
	merged := before + after

	var result []string
	result = append(result, lines[:sLine]...)
	result = append(result, merged)
	result = append(result, lines[eLine+1:]...)
	return strings.Join(result, "\n")
}

func wordLeftPos(content string, line, col int) (int, int) {
	lines := strings.Split(content, "\n")
	runes := []rune(lines[line])

	// If at start of line, go to end of previous line
	if col == 0 {
		if line > 0 {
			prevRunes := []rune(lines[line-1])
			return wordLeftPos(content, line-1, len(prevRunes))
		}
		return 0, 0
	}

	// Skip non-word chars going left
	pos := col - 1
	for pos > 0 && !isWordChar(runes[pos]) {
		pos--
	}
	// Skip word chars going left
	for pos > 0 && isWordChar(runes[pos-1]) {
		pos--
	}
	// If we were on a non-word char and hit start, check if it's a word char
	if pos == 0 && col > 0 && !isWordChar(runes[0]) {
		// We skipped non-word chars to 0, go to previous line
		if line > 0 {
			prevRunes := []rune(lines[line-1])
			return line - 1, len(prevRunes)
		}
	}
	return line, pos
}

func wordRightPos(content string, line, col int) (int, int) {
	lines := strings.Split(content, "\n")
	runes := []rune(lines[line])

	// If at end of line, go to start of next line
	if col >= len(runes) {
		if line < len(lines)-1 {
			return line + 1, 0
		}
		return line, len(runes)
	}

	pos := col
	// Skip word chars going right
	for pos < len(runes) && isWordChar(runes[pos]) {
		pos++
	}
	// Skip non-word chars going right
	for pos < len(runes) && !isWordChar(runes[pos]) {
		pos++
	}
	// If we reached end of line, stay there (next call jumps to next line)
	return line, pos
}

// --- SelectableEditor methods ---

func (e *SelectableEditor) cursorCol() int {
	return e.LineInfo().ColumnOffset
}

func (e *SelectableEditor) startSelectionIfNeeded() {
	if !e.selActive {
		e.selAnchorLine = e.Line()
		e.selAnchorCol = e.cursorCol()
		e.selActive = true
	}
}

func (e *SelectableEditor) ClearSelection() {
	e.selActive = false
}

func (e *SelectableEditor) SelectedText() string {
	if !e.selActive {
		return ""
	}
	sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
	return extractText(e.Value(), sL, sC, eL, eC)
}

func (e *SelectableEditor) DeleteSelection() {
	if !e.selActive {
		return
	}
	sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
	newContent := deleteText(e.Value(), sL, sC, eL, eC)
	e.SetValue(newContent)
	e.moveTo(sL, sC)
	e.selActive = false
}

func (e *SelectableEditor) ReplaceSelection(text string) {
	if !e.selActive {
		return
	}
	e.DeleteSelection()
	e.InsertString(text)
}

func (e *SelectableEditor) SelectAll() {
	lines := strings.Split(e.Value(), "\n")
	lastLine := len(lines) - 1
	lastCol := len([]rune(lines[lastLine]))
	e.selAnchorLine = 0
	e.selAnchorCol = 0
	e.moveTo(lastLine, lastCol)
	e.selActive = true
}

func (e *SelectableEditor) moveTo(line, col int) {
	// Move to correct line
	for e.Line() > line {
		e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	for e.Line() < line {
		e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	e.SetCursor(col)
}

func (e *SelectableEditor) moveCursorLeft() {
	e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyLeft})
}

func (e *SelectableEditor) moveCursorRight() {
	e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyRight})
}

func (e *SelectableEditor) moveCursorUp() {
	e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyUp})
}

func (e *SelectableEditor) moveCursorDown() {
	e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyDown})
}

func (e *SelectableEditor) moveWordLeft() {
	line, col := wordLeftPos(e.Value(), e.Line(), e.cursorCol())
	e.moveTo(line, col)
}

func (e *SelectableEditor) moveWordRight() {
	line, col := wordRightPos(e.Value(), e.Line(), e.cursorCol())
	e.moveTo(line, col)
}

func (e *SelectableEditor) copyToClipboard(text string) {
	e.textClip = text
	clipboard.WriteAll(text) // ignore error — internal clip is fallback
}

func (e *SelectableEditor) readFromClipboard() string {
	text, err := clipboard.ReadAll()
	if err == nil && text != "" {
		return text
	}
	return e.textClip
}

func (e *SelectableEditor) Copy() {
	if !e.selActive {
		return
	}
	e.copyToClipboard(e.SelectedText())
}

func (e *SelectableEditor) Cut() {
	if !e.selActive {
		return
	}
	e.copyToClipboard(e.SelectedText())
	e.DeleteSelection()
}

func (e *SelectableEditor) Paste() tea.Cmd {
	text := e.readFromClipboard()
	if text == "" {
		return nil
	}
	if e.selActive {
		e.DeleteSelection()
	}
	e.InsertString(text)
	return nil
}

// HandleKey processes key events, handling selection, word-jump, and clipboard
// operations. Non-selection keys are delegated to the underlying textarea.
func (e *SelectableEditor) HandleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	switch key {
	// --- Selection movement ---
	case "shift+left":
		e.startSelectionIfNeeded()
		e.moveCursorLeft()
		return nil
	case "shift+right":
		e.startSelectionIfNeeded()
		e.moveCursorRight()
		return nil
	case "shift+up":
		e.startSelectionIfNeeded()
		e.moveCursorUp()
		return nil
	case "shift+down":
		e.startSelectionIfNeeded()
		e.moveCursorDown()
		return nil
	case "shift+home":
		e.startSelectionIfNeeded()
		e.SetCursor(0)
		return nil
	case "shift+end":
		e.startSelectionIfNeeded()
		e.CursorEnd()
		return nil

	// --- Word jump (no selection) ---
	case "ctrl+left":
		e.ClearSelection()
		e.moveWordLeft()
		return nil
	case "ctrl+right":
		e.ClearSelection()
		e.moveWordRight()
		return nil

	// --- Word select ---
	case "ctrl+shift+left":
		e.startSelectionIfNeeded()
		e.moveWordLeft()
		return nil
	case "ctrl+shift+right":
		e.startSelectionIfNeeded()
		e.moveWordRight()
		return nil

	// --- Select all ---
	case "ctrl+a":
		e.SelectAll()
		return nil

	// --- Clipboard ---
	case "ctrl+c":
		e.Copy()
		return nil
	case "ctrl+x":
		e.Cut()
		return nil
	case "ctrl+v":
		return e.Paste()

	// --- Delete with selection ---
	case "backspace", "delete":
		if e.selActive {
			e.DeleteSelection()
			return nil
		}
		// Fall through to textarea
	}

	// Printable char with active selection: replace selection
	if e.selActive && msg.Type == tea.KeyRunes {
		e.DeleteSelection()
		// Fall through to textarea to insert the typed character
	} else if e.selActive {
		// Non-modifying key clears selection
		switch msg.Type {
		case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
			e.ClearSelection()
		}
	}

	// Delegate to textarea
	var cmd tea.Cmd
	e.Model, cmd = e.Model.Update(msg)
	return cmd
}

// View renders the editor. When selection is active, renders with custom
// highlighting. Otherwise delegates to the textarea's View.
func (e SelectableEditor) View() string {
	if !e.selActive {
		return e.Model.View()
	}
	return e.renderWithSelection()
}

func (e *SelectableEditor) renderWithSelection() string {
	content := e.Value()
	lines := strings.Split(content, "\n")
	curLine := e.Line()
	height := e.height
	if height <= 0 {
		height = 10
	}

	// Adjust viewport to keep cursor visible
	if curLine < e.viewOffset {
		e.viewOffset = curLine
	}
	if curLine >= e.viewOffset+height {
		e.viewOffset = curLine - height + 1
	}

	sLine, sCol, eLine, eCol := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())

	// Line number width
	numWidth := len(fmt.Sprintf("%d", len(lines)))
	if numWidth < 2 {
		numWidth = 2
	}

	var b strings.Builder
	endIdx := e.viewOffset + height
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	for i := e.viewOffset; i < endIdx; i++ {
		// Line number
		lineNum := fmt.Sprintf("%*d", numWidth, i+1)
		b.WriteString(lineNumberStyle.Render(lineNum))
		b.WriteString(" ")

		// Line content with selection highlighting
		runes := []rune(lines[i])
		for j, r := range runes {
			if posInRange(i, j, sLine, sCol, eLine, eCol) {
				b.WriteString(selectionStyle.Render(string(r)))
			} else {
				b.WriteString(string(r))
			}
		}

		if i < endIdx-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines
	for i := endIdx - e.viewOffset; i < height; i++ {
		b.WriteString("\n")
	}

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -run "TestIsWordChar|TestWordLeftPos|TestWordRightPos|TestSelectionRange|TestExtractText|TestDeleteText|TestPosInRange" -v
```

Expected: all PASS.

- [ ] **Step 5: Run full build**

```bash
go build ./...
```

Expected: compiles. (Note: `SelectableEditor` is defined but not yet used in model.go — that happens in Task 4.)

- [ ] **Step 6: Commit**

```bash
git add selection.go selection_test.go
git commit -m "feat: add SelectableEditor with word boundaries and selection helpers"
```

---

### Task 4: Integrate SelectableEditor into model.go

**Files:**
- Modify: `editor.go`
- Modify: `model.go`
- Modify: `plugin_diff.go`

- [ ] **Step 1: Update editor.go**

Replace `newEditor()` with `newSelectableEditor()` and update `setEditorSize`:

```go
// editor.go
package main

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

var editorStyle = lipgloss.NewStyle().Padding(0, 1)

var lineNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

func newSelectableEditor() SelectableEditor {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = true
	ta.FocusedStyle.LineNumber = lineNumberStyle
	ta.BlurredStyle.LineNumber = lineNumberStyle
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.Focus()
	return SelectableEditor{
		Model:         ta,
		selAnchorLine: -1,
	}
}

func setEditorSize(e *SelectableEditor, width, height int) {
	e.SetWidth(width - 2)
	e.SetHeight(height)
	e.height = height
}

func editorCursorPos(e SelectableEditor) (line, col int) {
	return e.Line(), e.LineInfo().ColumnOffset
}
```

- [ ] **Step 2: Update model struct and constructor in model.go**

Change the `editor` field type from `textarea.Model` to `SelectableEditor`:
```go
	editor       SelectableEditor
```

In `newModel()`, change `newEditor()` to `newSelectableEditor()`:
```go
	m := model{
		vault:              vault,
		activePanel:        treePanel,
		editorMode:         modeEdit,
		editor:             newSelectableEditor(),
		// ... rest unchanged
	}
```

- [ ] **Step 3: Update Update() non-key message path**

Change the non-key update path at the bottom of `Update()` from:
```go
	if m.activePanel == editorPanel && m.editorMode == modeEdit {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
	}
```
to:
```go
	if m.activePanel == editorPanel && m.editorMode == modeEdit {
		var cmd tea.Cmd
		m.editor.Model, cmd = m.editor.Model.Update(msg)
		cmds = append(cmds, cmd)
	}
```

- [ ] **Step 4: Update handleEditorKeys to use SelectableEditor**

Replace the last block in `handleEditorKeys` (the textarea delegation):
```go
	// Before (old code):
	// var cmd tea.Cmd
	// m.editor, cmd = m.editor.Update(msg)
	// return m, cmd

	// After:
	cmd := m.editor.HandleKey(msg)
	return m, cmd
```

Also add `m.editor.ClearSelection()` at the start of the esc handler in `handleEditorKeys`.

- [ ] **Step 5: Handle ctrl+c/x/v context in global Update handler**

In `Update()`, in the `tea.KeyMsg` switch, add these cases (after removing `"ctrl+c"` from quit in Task 2):

```go
		case "ctrl+c":
			if m.activePanel == treePanel {
				node := m.tree.selectedNode()
				if node != nil && !node.IsDir {
					m.fileClip = fileClipboard{path: node.Path, op: clipCopy}
					m.errMsg = "Copied: " + node.Name
				}
				return m, nil
			}
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				m.editor.Copy()
			}
			return m, nil

		case "ctrl+x":
			if m.activePanel == treePanel {
				node := m.tree.selectedNode()
				if node != nil && !node.IsDir {
					m.fileClip = fileClipboard{path: node.Path, op: clipCut}
					m.tree.cutPath = node.Path
					m.errMsg = "Cut: " + node.Name
				}
				return m, nil
			}
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				m.editor.Cut()
			}
			return m, nil

		case "ctrl+v":
			if m.activePanel == treePanel {
				if !m.fileClip.empty() {
					m.pasteFile()
				}
				return m, nil
			}
			if m.activePanel == editorPanel && m.editorMode == modeEdit {
				m.editor.Paste()
			}
			return m, nil
```

Then remove the duplicate `ctrl+x`, `ctrl+c`, `ctrl+v` cases from `handleTreeKeys` (added in Task 2) since they're now handled globally. Keep the `pasteFile()` method.

- [ ] **Step 6: Update plugin_diff.go to use SelectableEditor**

In `handlePluginDiff`, the accept case uses `m.editor.SetValue()`. Since `SetValue` is inherited from the embedded textarea.Model, no change is needed. However, add `m.editor.ClearSelection()` after setting the value:

```go
	case "y":
		m.editor.SetValue(m.pluginDiffResult)
		m.editor.ClearSelection()
		// ... rest unchanged
```

- [ ] **Step 7: Update setEditorSize and editorCursorPos calls**

In `model.go`, update `recalcLayout()`:
```go
	// Change from:
	// setEditorSize(&m.editor, m.editorWidth, m.editorHeight)
	// To:
	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)
	// (Same syntax — works because editor is now SelectableEditor and setEditorSize takes *SelectableEditor)
```

Update `View()` editorCursorPos call:
```go
	// Change from: line, col := editorCursorPos(m.editor)
	line, col := editorCursorPos(m.editor)
	// (Same syntax — editorCursorPos now takes SelectableEditor)
```

- [ ] **Step 8: Run build and tests**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.

- [ ] **Step 9: Commit**

```bash
git add editor.go model.go selection.go plugin_diff.go
git commit -m "feat: integrate SelectableEditor with text selection and clipboard"
```

---

### Task 5: AI shortcuts — data model and persistence

**Files:**
- Create: `shortcuts.go`
- Create: `shortcuts_test.go`
- Modify: `plugin_openrouter.go`

- [ ] **Step 1: Write tests for shortcut load/save**

```go
// shortcuts_test.go
package main

import (
	"testing"
)

func TestSaveAndLoadShortcuts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	shortcuts := []AIShortcut{
		{Name: "Fix grammar", Prompt: "Fix grammar errors"},
		{Name: "Summarize", Prompt: "Summarize this text"},
	}
	if err := saveShortcuts(shortcuts); err != nil {
		t.Fatalf("saveShortcuts() error: %v", err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d shortcuts, want 2", len(loaded))
	}
	if loaded[0].Name != "Fix grammar" {
		t.Errorf("first shortcut name = %q, want %q", loaded[0].Name, "Fix grammar")
	}
	if loaded[1].Prompt != "Summarize this text" {
		t.Errorf("second shortcut prompt = %q, want %q", loaded[1].Prompt, "Summarize this text")
	}
}

func TestLoadShortcuts_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() should not error for missing file: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty slice, got %d shortcuts", len(loaded))
	}
}

func TestShortcutsPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := shortcutsPath()
	want := "/tmp/test-xdg/clipad/ai_shortcuts.toml"
	if got != want {
		t.Errorf("shortcutsPath() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -run "TestSaveAndLoadShortcuts|TestLoadShortcuts_Missing|TestShortcutsPath" -v
```

Expected: FAIL.

- [ ] **Step 3: Extract callOpenRouter helper from plugin_openrouter.go**

Refactor `plugin_openrouter.go` to extract the HTTP call into a shared function:

Add this function to `plugin_openrouter.go`:
```go
func callOpenRouter(url, apiKey, model, systemPrompt, userMessage string) (string, error) {
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}
```

Then simplify the `Run` method to delegate:
```go
func (p *OpenRouterPlugin) Run(content string, prompt string, config map[string]string) (string, error) {
	url := p.BaseURL
	if url == "" {
		url = defaultOpenRouterURL
	}

	systemPrompt := "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations."
	userMessage := fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content)
	return callOpenRouter(url, config["api_key"], config["model"], systemPrompt, userMessage)
}
```

- [ ] **Step 4: Implement shortcuts.go**

```go
// shortcuts.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	toml "github.com/pelletier/go-toml/v2"
)

type AIShortcut struct {
	Name   string `toml:"name"`
	Prompt string `toml:"prompt"`
}

type aiShortcutsConfig struct {
	Shortcuts []AIShortcut `toml:"shortcuts"`
}

type shortcutResultMsg struct {
	result string
	err    error
}

func shortcutsPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "ai_shortcuts.toml")
}

func loadShortcuts() ([]AIShortcut, error) {
	data, err := os.ReadFile(shortcutsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading shortcuts: %w", err)
	}
	var cfg aiShortcutsConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing shortcuts: %w", err)
	}
	return cfg.Shortcuts, nil
}

func saveShortcuts(shortcuts []AIShortcut) error {
	path := shortcutsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating shortcuts dir: %w", err)
	}
	cfg := aiShortcutsConfig{Shortcuts: shortcuts}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling shortcuts: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func runShortcutCmd(shortcut AIShortcut, content string, config map[string]string) tea.Cmd {
	return func() tea.Msg {
		systemPrompt := "You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."
		userMessage := fmt.Sprintf("Instruction: %s\n\nText:\n%s", shortcut.Prompt, content)
		result, err := callOpenRouter(defaultOpenRouterURL, config["api_key"], config["model"], systemPrompt, userMessage)
		return shortcutResultMsg{result: result, err: err}
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./...
```

Expected: all tests pass including new shortcut tests and existing OpenRouter tests (refactor preserved behavior).

- [ ] **Step 6: Commit**

```bash
git add shortcuts.go shortcuts_test.go plugin_openrouter.go
git commit -m "feat: add AI shortcuts data model with TOML persistence"
```

---

### Task 6: AI shortcuts — UI, execution, and integration

**Files:**
- Create: `shortcuts_modal.go`
- Create: `shortcuts_input.go`
- Modify: `model.go`

- [ ] **Step 1: Create shortcuts_modal.go**

```go
// shortcuts_modal.go
package main

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	shortcutItemStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	shortcutCursorStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	shortcutEmptyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		PaddingLeft(2)

	shortcutHintStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		PaddingLeft(2)
)

func shortcutSelectorView(shortcuts []AIShortcut, cursor int, width, height int) string {
	if len(shortcuts) == 0 {
		content := shortcutEmptyStyle.Render("No shortcuts. Press Ctrl+L to create one.")
		return statusBarStyle.Width(width).Render(content)
	}

	var items string
	for i, s := range shortcuts {
		name := s.Name
		if i == cursor {
			name = shortcutCursorStyle.Render("> " + name)
		} else {
			name = shortcutItemStyle.Render("  " + name)
		}
		if i > 0 {
			items += "\n"
		}
		items += name
	}

	hint := shortcutHintStyle.Render("Enter:run  e:edit  d:delete  Esc:close")
	content := items + "\n" + hint

	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(content)
}
```

- [ ] **Step 2: Create shortcuts_input.go**

```go
// shortcuts_input.go
package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleShortcutSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.shortcutCursor > 0 {
			m.shortcutCursor--
		}
	case "down", "j":
		if m.shortcutCursor < len(m.shortcuts)-1 {
			m.shortcutCursor++
		}
	case "enter":
		if len(m.shortcuts) == 0 || m.shortcutCursor >= len(m.shortcuts) {
			return m, nil
		}
		shortcut := m.shortcuts[m.shortcutCursor]
		// Check OpenRouter config
		cfg, err := loadPluginConfig("openrouter")
		if err != nil || !pluginConfigComplete((&OpenRouterPlugin{}).ConfigFields(), cfg) {
			// Need to configure OpenRouter first
			m.pluginActive = &OpenRouterPlugin{}
			m.pluginConfigFields = m.pluginActive.ConfigFields()
			m.pluginConfigIndex = 0
			m.pluginConfigValues = make(map[string]string)
			m.inputMode = inputPluginConfig
			m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
			return m, textinput.Blink
		}
		// Determine content: selected text or full file
		content := m.editor.Value()
		m.shortcutOnSelection = m.editor.selActive
		if m.shortcutOnSelection {
			content = m.editor.SelectedText()
		}
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runShortcutCmd(shortcut, content, cfg)
	case "e":
		if len(m.shortcuts) > 0 && m.shortcutCursor < len(m.shortcuts) {
			m.shortcutEditing = m.shortcutCursor
			m.inputMode = inputShortcutName
			m.shortcutNameInput.SetValue(m.shortcuts[m.shortcutCursor].Name)
			cmd := m.shortcutNameInput.Focus()
			return m, cmd
		}
	case "d":
		if len(m.shortcuts) > 0 && m.shortcutCursor < len(m.shortcuts) {
			m.inputMode = inputShortcutDeleteConfirm
		}
	case "esc":
		m.inputMode = inputNone
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m model) handleShortcutName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := m.shortcutNameInput.Value()
		if name == "" {
			return m, nil
		}
		m.shortcutTempName = name
		m.inputMode = inputShortcutPrompt
		if m.shortcutEditing >= 0 {
			m.shortcutPromptInput.SetValue(m.shortcuts[m.shortcutEditing].Prompt)
		} else {
			m.shortcutPromptInput.SetValue("")
		}
		cmd := m.shortcutPromptInput.Focus()
		return m, cmd
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutNameInput, cmd = m.shortcutNameInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := m.shortcutPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		shortcut := AIShortcut{Name: m.shortcutTempName, Prompt: prompt}
		if m.shortcutEditing >= 0 {
			m.shortcuts[m.shortcutEditing] = shortcut
		} else {
			m.shortcuts = append(m.shortcuts, shortcut)
		}
		if err := saveShortcuts(m.shortcuts); err != nil {
			m.errMsg = "Failed to save shortcuts: " + err.Error()
		}
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutPromptInput, cmd = m.shortcutPromptInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		if m.shortcutCursor < len(m.shortcuts) {
			m.shortcuts = append(m.shortcuts[:m.shortcutCursor], m.shortcuts[m.shortcutCursor+1:]...)
			if err := saveShortcuts(m.shortcuts); err != nil {
				m.errMsg = "Failed to save shortcuts: " + err.Error()
			}
			if m.shortcutCursor >= len(m.shortcuts) && m.shortcutCursor > 0 {
				m.shortcutCursor--
			}
		}
		if len(m.shortcuts) == 0 {
			m.inputMode = inputNone
		} else {
			m.inputMode = inputShortcutSelect
		}
	case "n", "esc":
		m.inputMode = inputShortcutSelect
	}
	return m, nil
}
```

- [ ] **Step 3: Add new input modes and fields to model.go**

Add to the `inputMode` enum:
```go
	inputShortcutSelect
	inputShortcutName
	inputShortcutPrompt
	inputShortcutDeleteConfirm
```

Add fields to the model struct:
```go
	// AI shortcuts
	shortcuts           []AIShortcut
	shortcutCursor      int
	shortcutEditing     int // -1 for new, >= 0 for editing index
	shortcutTempName    string
	shortcutOnSelection bool // true if shortcut was run on selected text (not full file)
	shortcutNameInput   textinput.Model
	shortcutPromptInput textinput.Model
```

In `newModel()`, initialize the new inputs:
```go
	sn := textinput.New()
	sn.Placeholder = "shortcut name"
	sn.CharLimit = 256

	sp := textinput.New()
	sp.Placeholder = "prompt template"
	sp.CharLimit = 500
```

And add to the model initializer:
```go
		shortcutNameInput:   sn,
		shortcutPromptInput: sp,
		shortcutEditing:     -1,
```

Load shortcuts in `newModel()`:
```go
	shortcuts, _ := loadShortcuts()
	m.shortcuts = shortcuts
```

- [ ] **Step 4: Add shortcut key handlers to global Update**

In `Update()`, in the `tea.KeyMsg` switch, add:
```go
		case "ctrl+g":
			if m.currentFile != "" || m.newNoteDir != "" {
				m.shortcuts, _ = loadShortcuts() // refresh
				m.inputMode = inputShortcutSelect
				m.shortcutCursor = 0
			}
			return m, nil

		case "ctrl+l":
			if m.currentFile != "" || m.newNoteDir != "" {
				m.shortcutEditing = -1
				m.inputMode = inputShortcutName
				m.shortcutNameInput.SetValue("")
				cmd := m.shortcutNameInput.Focus()
				return m, cmd
			}
			return m, nil
```

- [ ] **Step 5: Wire up shortcut input mode dispatching**

In `handleInputMode()`, add cases:
```go
	case inputShortcutSelect:
		return m.handleShortcutSelect(msg)
	case inputShortcutName:
		return m.handleShortcutName(msg)
	case inputShortcutPrompt:
		return m.handleShortcutPrompt(msg)
	case inputShortcutDeleteConfirm:
		return m.handleShortcutDeleteConfirm(msg)
```

- [ ] **Step 6: Handle shortcutResultMsg in Update**

In `Update()`, add a message handler (near the existing `pluginResultMsg` handler):
```go
	case shortcutResultMsg:
		m.pluginProcessing = false
		if msg.err != nil {
			m.errMsg = "Shortcut error: " + msg.err.Error()
			m.inputMode = inputNone
			m.pluginDiffOriginal = ""
			return m, nil
		}
		if msg.result == m.pluginDiffOriginal {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginDiffOriginal = ""
			return m, nil
		}
		m.pluginDiffResult = msg.result
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, msg.result, m.editorWidth, m.editorHeight)
		m.inputMode = inputPluginDiff
		return m, nil
```

In `handlePluginDiff`, for the "y" (accept) case, handle selection-based replacement. Replace the existing accept logic:

```go
	case "y":
		if m.shortcutOnSelection {
			// Shortcut was run on selected text — replace just the selection
			m.editor.ReplaceSelection(m.pluginDiffResult)
			m.shortcutOnSelection = false
		} else {
			m.editor.SetValue(m.pluginDiffResult)
		}
		m.editor.ClearSelection()
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
```

- [ ] **Step 7: Add status bar rendering for shortcut modes**

In `View()`, add rendering for the new input modes (insert these in the statusView chain before the final `if m.inputMode == inputPluginSelect` block):

```go
	} else if m.inputMode == inputShortcutName {
		statusView = statusBarStyle.Width(m.width).Render(
			"Shortcut name: " + m.shortcutNameInput.View())
	} else if m.inputMode == inputShortcutPrompt {
		statusView = statusBarStyle.Width(m.width).Render(
			"Prompt: " + m.shortcutPromptInput.View())
	} else if m.inputMode == inputShortcutDeleteConfirm {
		name := ""
		if m.shortcutCursor < len(m.shortcuts) {
			name = m.shortcuts[m.shortcutCursor].Name
		}
		statusView = statusBarStyle.Width(m.width).Render(
			fmt.Sprintf("Delete shortcut %q? (y/n)", name))
```

For the shortcut selector, render it over the right panel (editor area) as a modal overlay. In `View()`, add a check in the right panel rendering section (near the `inputPluginDiff` check):

```go
	} else if m.inputMode == inputShortcutSelect {
		rightView = shortcutSelectorView(m.shortcuts, m.shortcutCursor, m.editorWidth, m.editorHeight)
```

- [ ] **Step 8: Update status bar hints**

In `statusbar.go`, add shortcut hints to the StatusBar. In `StatusBar.View()`, add after the plugins hint:

```go
	if s.fileOpen {
		hints = append(hints, hint{"^Spc", "plugins"}, hint{"^G", "AI"})
	}
```

- [ ] **Step 9: Run build and tests**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.

- [ ] **Step 10: Commit**

```bash
git add shortcuts_input.go shortcuts_modal.go model.go statusbar.go
git commit -m "feat: add AI shortcuts with context menu, CRUD, and LLM execution"
```

---

## Post-Implementation Verification

After all tasks are complete, run the full test suite and build:

```bash
go test ./... -v
go build -o clipad .
```

Then manually verify each feature:
1. **Auto-save:** Open a file, make changes, wait 15s — should see "Auto-saved" flash
2. **File clipboard:** Select a file in tree, Ctrl+X to cut, navigate to folder, Ctrl+V to paste
3. **Text selection:** In editor, Shift+Right to select characters, Ctrl+C to copy, Ctrl+V to paste
4. **Word jump:** Ctrl+Left/Right to jump by word
5. **AI shortcuts:** Ctrl+L to create a shortcut, Ctrl+G to open menu and run it
6. **Ctrl+Q:** Still quits the app (Ctrl+C no longer quits)
