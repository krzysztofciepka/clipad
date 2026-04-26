package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// hitTestPanel maps a terminal mouse coordinate to the panel it landed in.
// The status-bar row and any out-of-bounds coordinates return ok=false.
// When treeWidth == 0 (narrow terminal), the full width is treated as editor.
// When chatWidth > 0 (chat panel open), the rightmost chatWidth columns
// (after a 1-column border) hit the chat panel.
func hitTestPanel(treeWidth, chatWidth, width, height, x, y int) (hit panel, localX, localY int, ok bool) {
	if x < 0 || y < 0 || x >= width || y >= height {
		return 0, 0, 0, false
	}
	if y >= height-1 {
		return 0, 0, 0, false
	}
	// Chat region (rightmost): treeWidth + treeBorder + editorWidth + chatBorder + chatWidth = width
	// Chat starts at width - chatWidth; the column at width - chatWidth - 1 is the border.
	if chatWidth > 0 {
		chatStart := width - chatWidth
		if x >= chatStart {
			return chatPanelHit, x - chatStart, y, true
		}
		if x == chatStart-1 {
			return 0, 0, 0, false // chat-left border
		}
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

// wrapRow describes one visual row in a wrapped view of the content.
type wrapRow struct {
	line     int // logical line index
	startCol int // first column (rune index) of this wrap row within the line
	length   int // number of runes on this wrap row
}

// wrapLineStarts returns the rune indices where a line soft-wraps. Greedy
// word-wrap: breaks at the last space inside a `width`-rune window; falls
// back to a hard break at `width` when no space is available (very long
// words). The first entry is always 0.
func wrapLineStarts(runes []rune, width int) []int {
	if width <= 0 || len(runes) == 0 {
		return []int{0}
	}
	starts := []int{0}
	cursor := 0
	for cursor+width < len(runes) {
		breakAt := -1
		for i := cursor + width - 1; i > cursor; i-- {
			if runes[i] == ' ' || runes[i] == '\t' {
				breakAt = i + 1
				break
			}
		}
		if breakAt <= cursor {
			breakAt = cursor + width
		}
		starts = append(starts, breakAt)
		cursor = breakAt
	}
	return starts
}

// wrapContent builds the visual-row layout for content at wrapWidth. Both
// the renderer and the click translation derive their positions from this,
// so they stay in sync regardless of the wrap rule used.
func wrapContent(content string, wrapWidth int) []wrapRow {
	lines := strings.Split(content, "\n")
	var rows []wrapRow
	for li, ln := range lines {
		runes := []rune(ln)
		if len(runes) == 0 {
			rows = append(rows, wrapRow{line: li})
			continue
		}
		starts := wrapLineStarts(runes, wrapWidth)
		for idx, start := range starts {
			end := len(runes)
			if idx+1 < len(starts) {
				end = starts[idx+1]
			}
			rows = append(rows, wrapRow{line: li, startCol: start, length: end - start})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, wrapRow{line: 0})
	}
	return rows
}

// cursorVisualRow returns the visual-row index of a cursor at logical
// (line, col) in the wrapped layout.
func cursorVisualRow(content string, line, col, wrapWidth int) int {
	rows := wrapContent(content, wrapWidth)
	for i, r := range rows {
		if r.line != line {
			continue
		}
		// last wrap row of this line absorbs col == startCol+length
		isLastOfLine := i == len(rows)-1 || rows[i+1].line != line
		if col >= r.startCol && col < r.startCol+r.length {
			return i
		}
		if isLastOfLine && col == r.startCol+r.length {
			return i
		}
	}
	return 0
}

// mousePosToEditorCursor translates panel-local coordinates to a (line, col)
// position in the editor content. Accounts for editorStyle's Padding(0, 1)
// left padding, the line-number column plus its trailing space, the
// textarea's visual scroll offset, and the wrap layout produced by
// wrapContent.
func mousePosToEditorCursor(content string, visualYOffset, localX, localY, numWidth, wrapWidth int) (line, col int) {
	rows := wrapContent(content, wrapWidth)
	if len(rows) == 0 {
		return 0, 0
	}
	visualRow := visualYOffset + localY
	if visualRow < 0 {
		visualRow = 0
	}
	if visualRow >= len(rows) {
		visualRow = len(rows) - 1
	}
	r := rows[visualRow]

	col = localX - numWidth - 2
	if col < 0 {
		col = 0
	}
	col += r.startCol
	if col > r.startCol+r.length {
		col = r.startCol + r.length
	}
	return r.line, col
}

// mousePosToTreeRow translates local Y in the tree panel to an absolute row
// index. Caller must validate against len(items).
func mousePosToTreeRow(treeOffset, localY int) int {
	return treeOffset + localY
}

// handleEditorMouse routes a mouse event that landed in the editor panel.
// localX/localY are relative to the editor panel's top-left.
func handleEditorMouse(m model, localX, localY int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		numWidth := editorNumWidth(m.editor.Value())
		wrapWidth := m.editor.Width()
		line, col := mousePosToEditorCursor(m.editor.Value(), m.editor.visualYOffset, localX, localY, numWidth, wrapWidth)
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

// handleTreeMouse routes a mouse event that landed in the tree panel.
func handleTreeMouse(m model, localY int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// localY == 0 is the pinned Add note row.
		if localY == 0 {
			m.tree.cursor = -1
			m.activePanel = treePanel
			if m.isDirty() {
				m.inputMode = inputUnsavedGuard
				m.pendingAction = pendingNewNote
				return m, nil
			}
			m.startNewNote()
			return m, nil
		}
		// localY >= 1 maps to items[localY-1+offset].
		row := mousePosToTreeRow(m.tree.offset, localY-1)
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
		m.tree.scrollBy(-3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.tree.scrollBy(3)
		return m, nil
	}
	return m, nil
}

// handleChatMouse routes a mouse event that landed in the chat panel.
// Wheel events scroll the scrollback; clicks are otherwise ignored for now.
func handleChatMouse(m model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.chatViewport.LineUp(3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.chatViewport.LineDown(3)
		return m, nil
	}
	return m, nil
}

// handleMouseMsg is the top-level mouse dispatcher. Callers must ensure
// !m.pluginProcessing. inputMode must be inputNone except for inputHelp,
// which routes wheel events to the help viewport.
func handleMouseMsg(m model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.inputMode == inputHelp &&
		(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		var cmd tea.Cmd
		m.helpViewport, cmd = m.helpViewport.Update(msg)
		return m, cmd
	}
	hit, localX, localY, ok := hitTestPanel(m.treeWidth, m.chatWidth, m.width, m.height, msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	switch hit {
	case treePanel:
		return handleTreeMouse(m, localY, msg)
	case chatPanelHit:
		return handleChatMouse(m, msg)
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
