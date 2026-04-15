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
