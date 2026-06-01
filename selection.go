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
	height        int
	selActive     bool
	selAnchorLine int
	selAnchorCol  int
	textClip      string
	viewOffset    int
	mouseDragging bool
	visualYOffset int // mirrors textarea's internal viewport YOffset for click mapping
	history       editHistory
}

// --- Pure helper functions ---

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
	// Guard against stale selection coordinates pointing past a buffer that has
	// since been replaced with a shorter one (e.g. starting a new note).
	if sLine < 0 {
		sLine, sCol = 0, 0
	}
	if eLine > len(lines)-1 {
		eLine = len(lines) - 1
		eCol = len([]rune(lines[eLine]))
	}
	if sLine > eLine {
		return ""
	}
	if sLine == eLine {
		runes := []rune(lines[sLine])
		if eCol > len(runes) {
			eCol = len(runes)
		}
		return string(runes[sCol:eCol])
	}
	var b strings.Builder
	runes := []rune(lines[sLine])
	b.WriteString(string(runes[sCol:]))
	for i := sLine + 1; i < eLine; i++ {
		b.WriteByte('\n')
		b.WriteString(lines[i])
	}
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
	if col == 0 {
		if line > 0 {
			prevRunes := []rune(lines[line-1])
			return wordLeftPos(content, line-1, len(prevRunes))
		}
		return 0, 0
	}
	pos := col - 1
	for pos > 0 && !isWordChar(runes[pos]) {
		pos--
	}
	for pos > 0 && isWordChar(runes[pos-1]) {
		pos--
	}
	if pos == 0 && col > 0 && !isWordChar(runes[0]) {
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
	if col >= len(runes) {
		if line < len(lines)-1 {
			return line + 1, 0
		}
		return line, len(runes)
	}
	pos := col
	for pos < len(runes) && isWordChar(runes[pos]) {
		pos++
	}
	for pos < len(runes) && !isWordChar(runes[pos]) {
		pos++
	}
	return line, pos
}

// --- SelectableEditor methods ---

// cursorCol returns the cursor's LOGICAL column within its line (rune index
// from the start of the logical line). LineInfo().ColumnOffset alone is the
// offset within the current wrap row, not the logical line, so we add
// StartColumn back in.
func (e *SelectableEditor) cursorCol() int {
	info := e.LineInfo()
	return info.StartColumn + info.ColumnOffset
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
	pre := e.recordOp()
	sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
	newContent := deleteText(e.Value(), sL, sC, eL, eC)
	e.SetValue(newContent)
	e.moveTo(sL, sC)
	e.selActive = false
	e.commitOp(pre)
}

func (e *SelectableEditor) ReplaceSelection(text string) {
	if !e.selActive {
		return
	}
	pre := e.recordOp()
	sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
	newContent := deleteText(e.Value(), sL, sC, eL, eC)
	e.SetValue(newContent)
	e.moveTo(sL, sC)
	e.selActive = false
	e.InsertString(text)
	e.commitOp(pre)
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

// StartMouseDrag positions the cursor at (line, col) and records the click as
// a potential selection anchor. Selection is not yet active — only a
// subsequent UpdateMouseDrag to a different position activates it.
func (e *SelectableEditor) StartMouseDrag(line, col int) {
	e.moveTo(line, col)
	e.selAnchorLine = line
	e.selAnchorCol = col
	e.selActive = false
	e.mouseDragging = true
	e.syncVisualYOffset()
	e.noteMovement()
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
	e.syncVisualYOffset()
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
	e.syncVisualYOffset()
	e.noteMovement()
}

// ScrollDown moves cursor and viewport down by n lines.
func (e *SelectableEditor) ScrollDown(n int) {
	for i := 0; i < n; i++ {
		e.moveCursorDown()
	}
	e.adjustViewOffset()
	e.syncVisualYOffset()
	e.noteMovement()
}

// MoveTo positions the cursor at (line, col), both 0-indexed. Public
// wrapper around moveTo for callers in other files.
func (e *SelectableEditor) MoveTo(line, col int) {
	e.moveTo(line, col)
}

func (e *SelectableEditor) moveTo(line, col int) {
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
	clipboard.WriteAll(text)
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
	pre := e.recordOp()
	sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
	newContent := deleteText(e.Value(), sL, sC, eL, eC)
	e.SetValue(newContent)
	e.moveTo(sL, sC)
	e.selActive = false
	e.commitOp(pre)
}

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

// syncVisualYOffset mirrors bubbles' repositionView: keeps visualYOffset
// such that the cursor's visual row stays within [visualYOffset,
// visualYOffset+height). Call after any cursor movement so click
// translation stays in sync with the textarea's internal scroll.
func (e *SelectableEditor) syncVisualYOffset() {
	wrapWidth := e.Width()
	row := cursorVisualRow(e.Value(), e.Line(), e.cursorCol(), wrapWidth)
	if row < e.visualYOffset {
		e.visualYOffset = row
	}
	if e.height > 0 && row >= e.visualYOffset+e.height {
		e.visualYOffset = row - e.height + 1
	}
	if e.visualYOffset < 0 {
		e.visualYOffset = 0
	}
}

func (e *SelectableEditor) adjustViewOffset() {
	if e.height <= 0 {
		return
	}
	curLine := e.Line()
	if curLine < e.viewOffset {
		e.viewOffset = curLine
	}
	if curLine >= e.viewOffset+e.height {
		e.viewOffset = curLine - e.height + 1
	}
}

func (e *SelectableEditor) HandleKey(msg tea.KeyMsg) tea.Cmd {
	defer e.syncVisualYOffset()
	key := msg.String()

	switch key {
	case "ctrl+z":
		e.Undo()
		return nil
	case "ctrl+shift+z", "ctrl+y":
		e.Redo()
		return nil
	case "shift+left":
		e.startSelectionIfNeeded()
		e.moveCursorLeft()
		e.adjustViewOffset()
		e.noteMovement()
		return nil
	case "shift+right":
		e.startSelectionIfNeeded()
		e.moveCursorRight()
		e.adjustViewOffset()
		e.noteMovement()
		return nil
	case "shift+up":
		e.startSelectionIfNeeded()
		e.moveCursorUp()
		e.adjustViewOffset()
		e.noteMovement()
		return nil
	case "shift+down":
		e.startSelectionIfNeeded()
		e.moveCursorDown()
		e.adjustViewOffset()
		e.noteMovement()
		return nil
	case "shift+home":
		e.startSelectionIfNeeded()
		e.SetCursor(0)
		e.adjustViewOffset()
		e.noteMovement()
		return nil
	case "shift+end":
		e.startSelectionIfNeeded()
		e.CursorEnd()
		e.adjustViewOffset()
		e.noteMovement()
		return nil
	case "ctrl+left":
		e.ClearSelection()
		e.moveWordLeft()
		e.noteMovement()
		return nil
	case "ctrl+right":
		e.ClearSelection()
		e.moveWordRight()
		e.noteMovement()
		return nil
	case "ctrl+shift+left":
		e.startSelectionIfNeeded()
		e.moveWordLeft()
		e.noteMovement()
		return nil
	case "ctrl+shift+right":
		e.startSelectionIfNeeded()
		e.moveWordRight()
		e.noteMovement()
		return nil
	case "ctrl+a":
		e.SelectAll()
		e.noteMovement()
		return nil
	case "backspace", "delete":
		if e.selActive {
			e.DeleteSelection()
			return nil
		}
		pre := e.snapshotNow()
		pushed := e.history.recordBefore(editKindDeleting, pre)
		var cmd tea.Cmd
		e.Model, cmd = e.Model.Update(msg)
		if pushed && e.Value() == pre.content {
			e.history.revertLastPush()
		}
		return cmd
	}

	if e.selActive && msg.Type == tea.KeyRunes {
		// Merged delete-then-insert as a single undo entry. Inline the
		// selection delete so DeleteSelection doesn't record a separate op.
		pre := e.recordOp()
		sL, sC, eL, eC := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
		newContent := deleteText(e.Value(), sL, sC, eL, eC)
		e.SetValue(newContent)
		e.moveTo(sL, sC)
		e.selActive = false
		var cmd tea.Cmd
		e.Model, cmd = e.Model.Update(msg)
		e.commitOp(pre)
		return cmd
	} else if e.selActive {
		switch msg.Type {
		case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
			e.ClearSelection()
			e.noteMovement()
		}
	} else {
		switch msg.Type {
		case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
			e.noteMovement()
		}
	}

	switch msg.Type {
	case tea.KeyRunes, tea.KeyEnter, tea.KeySpace, tea.KeyTab:
		pre := e.snapshotNow()
		pushed := e.history.recordBefore(editKindTyping, pre)
		var cmd tea.Cmd
		e.Model, cmd = e.Model.Update(msg)
		if pushed && e.Value() == pre.content {
			e.history.revertLastPush()
		}
		return cmd
	}

	var cmd tea.Cmd
	e.Model, cmd = e.Model.Update(msg)
	return cmd
}

func (e *SelectableEditor) snapshotNow() snapshot {
	return snapshot{
		content:       e.Value(),
		line:          e.Line(),
		col:           e.cursorCol(),
		selActive:     e.selActive,
		selAnchorLine: e.selAnchorLine,
		selAnchorCol:  e.selAnchorCol,
	}
}

func (e *SelectableEditor) restoreSnapshot(s snapshot) {
	e.SetValue(s.content)
	e.moveTo(s.line, s.col)
	e.selActive = s.selActive
	e.selAnchorLine = s.selAnchorLine
	e.selAnchorCol = s.selAnchorCol
	e.syncVisualYOffset()
}

func (e *SelectableEditor) ClearHistory() {
	e.history.clear()
}

func (e *SelectableEditor) Undo() bool {
	pre, ok := e.history.popUndo()
	if !ok {
		return false
	}
	e.history.pushRedo(e.snapshotNow())
	e.restoreSnapshot(pre)
	e.history.breakGroup()
	return true
}

func (e *SelectableEditor) Redo() bool {
	next, ok := e.history.popRedo()
	if !ok {
		return false
	}
	e.history.pushUndo(e.snapshotNow())
	e.restoreSnapshot(next)
	e.history.breakGroup()
	return true
}

// noteMovement breaks the current edit group so the next edit starts a new
// one. Called on cursor movement, mouse click, scroll, and file switch.
func (e *SelectableEditor) noteMovement() {
	e.history.breakGroup()
}

// recordOp pushes a pre-edit snapshot for a single-entry op group (paste,
// cut, delete-selection, replace-selection, and model-level ops). Returns
// the snapshot that was pushed, so callers can verify the op changed content.
func (e *SelectableEditor) recordOp() snapshot {
	pre := e.snapshotNow()
	e.history.recordBefore(editKindOp, pre)
	return pre
}

// commitOp pops the op's snapshot back off if the buffer content is unchanged
// (i.e. the op turned out to be a no-op).
func (e *SelectableEditor) commitOp(pre snapshot) {
	if e.Value() == pre.content {
		e.history.revertLastPush()
	}
}

var cursorStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("252")).
	Foreground(lipgloss.Color("235"))

func (e SelectableEditor) View() string {
	return e.render()
}

// render draws the editor ourselves, handling both selection and
// non-selection cases. Soft-wrap is done by wrapContent (word-wrap with
// char-break fallback); click translation uses the same function so
// visual rows and logical cursor positions stay in sync.
func (e SelectableEditor) render() string {
	content := e.Value()
	lines := strings.Split(content, "\n")
	height := e.height
	if height <= 0 {
		height = 10
	}
	wrapWidth := e.Width()
	if wrapWidth <= 0 {
		wrapWidth = 80
	}

	sLine, sCol, eLine, eCol := 0, 0, 0, 0
	if e.selActive {
		sLine, sCol, eLine, eCol = selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())
	}
	cursorLine := e.Line()
	cursorCol := e.cursorCol()

	numWidth := len(fmt.Sprintf("%d", len(lines)))
	if numWidth < 2 {
		numWidth = 2
	}

	rows := wrapContent(content, wrapWidth)

	var b strings.Builder
	endIdx := e.visualYOffset + height
	if endIdx > len(rows) {
		endIdx = len(rows)
	}
	drawnRows := 0

	for i := e.visualYOffset; i < endIdx; i++ {
		r := rows[i]
		if r.startCol == 0 {
			b.WriteString(lineNumberStyle.Render(fmt.Sprintf("%*d", numWidth, r.line+1)))
		} else {
			b.WriteString(lineNumberStyle.Render(strings.Repeat(" ", numWidth)))
		}
		b.WriteString(" ")

		var runes []rune
		if r.line < len(lines) {
			runes = []rune(lines[r.line])
		}
		end := r.startCol + r.length
		if end > len(runes) {
			end = len(runes)
		}
		for col := r.startCol; col < end; col++ {
			ch := runes[col]
			inSel := e.selActive && posInRange(r.line, col, sLine, sCol, eLine, eCol)
			isCursor := !e.selActive && r.line == cursorLine && col == cursorCol
			s := string(ch)
			switch {
			case isCursor:
				b.WriteString(cursorStyle.Render(s))
			case inSel:
				b.WriteString(selectionStyle.Render(s))
			default:
				b.WriteString(s)
			}
		}

		// Trailing cursor at or past end of line, drawn on the last wrap
		// row of the cursor's logical line.
		if !e.selActive && r.line == cursorLine {
			isLastOfLine := i == len(rows)-1 || rows[i+1].line != r.line
			if isLastOfLine && cursorCol >= end {
				b.WriteString(cursorStyle.Render(" "))
			}
		}

		b.WriteString("\n")
		drawnRows++
	}

	for i := drawnRows; i < height; i++ {
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}
