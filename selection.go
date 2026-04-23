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
	key := msg.String()

	switch key {
	case "shift+left":
		e.startSelectionIfNeeded()
		e.moveCursorLeft()
		e.adjustViewOffset()
		return nil
	case "shift+right":
		e.startSelectionIfNeeded()
		e.moveCursorRight()
		e.adjustViewOffset()
		return nil
	case "shift+up":
		e.startSelectionIfNeeded()
		e.moveCursorUp()
		e.adjustViewOffset()
		return nil
	case "shift+down":
		e.startSelectionIfNeeded()
		e.moveCursorDown()
		e.adjustViewOffset()
		return nil
	case "shift+home":
		e.startSelectionIfNeeded()
		e.SetCursor(0)
		e.adjustViewOffset()
		return nil
	case "shift+end":
		e.startSelectionIfNeeded()
		e.CursorEnd()
		e.adjustViewOffset()
		return nil
	case "ctrl+left":
		e.ClearSelection()
		e.moveWordLeft()
		return nil
	case "ctrl+right":
		e.ClearSelection()
		e.moveWordRight()
		return nil
	case "ctrl+shift+left":
		e.startSelectionIfNeeded()
		e.moveWordLeft()
		return nil
	case "ctrl+shift+right":
		e.startSelectionIfNeeded()
		e.moveWordRight()
		return nil
	case "ctrl+a":
		e.SelectAll()
		return nil
	case "backspace", "delete":
		if e.selActive {
			e.DeleteSelection()
			return nil
		}
	}

	if e.selActive && msg.Type == tea.KeyRunes {
		e.DeleteSelection()
	} else if e.selActive {
		switch msg.Type {
		case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
			e.ClearSelection()
		}
	}

	var cmd tea.Cmd
	e.Model, cmd = e.Model.Update(msg)
	return cmd
}

func (e SelectableEditor) View() string {
	if !e.selActive {
		return e.Model.View()
	}
	return e.renderWithSelection()
}

func (e SelectableEditor) renderWithSelection() string {
	content := e.Value()
	lines := strings.Split(content, "\n")
	height := e.height
	if height <= 0 {
		height = 10
	}

	// viewOffset is maintained by adjustViewOffset() in HandleKey
	sLine, sCol, eLine, eCol := selectionRange(e.selAnchorLine, e.selAnchorCol, e.Line(), e.cursorCol())

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
		lineNum := fmt.Sprintf("%*d", numWidth, i+1)
		b.WriteString(lineNumberStyle.Render(lineNum))
		b.WriteString(" ")

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

	for i := endIdx - e.viewOffset; i < height; i++ {
		b.WriteString("\n")
	}

	return b.String()
}
