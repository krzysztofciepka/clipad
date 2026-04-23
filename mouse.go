package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
