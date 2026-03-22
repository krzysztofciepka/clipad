package main

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

var editorStyle = lipgloss.NewStyle().Padding(0, 1)

func newEditor() textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = true
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.CharLimit = 0 // no limit
	ta.Focus()
	return ta
}

func setEditorSize(ta *textarea.Model, width, height int) {
	ta.SetWidth(width - 2)  // account for padding
	ta.SetHeight(height)
}

func editorCursorPos(ta textarea.Model) (line, col int) {
	return ta.Line(), ta.LineInfo().ColumnOffset
}
