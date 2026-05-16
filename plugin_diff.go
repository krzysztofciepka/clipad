package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	diffHeaderOriginalStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("196")).
				Padding(0, 1)

	diffHeaderNewStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("76")).
				Padding(0, 1)

	diffBorderStyle = lipgloss.NewStyle().
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder())
)

func (m model) handlePluginDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		if m.aiRunOnSelection {
			// ReplaceSelection records its own op entry.
			m.editor.ReplaceSelection(m.pluginDiffResult)
			m.aiRunOnSelection = false
		} else {
			pre := m.editor.recordOp()
			m.editor.SetValue(m.pluginDiffResult)
			m.editor.commitOp(pre)
		}
		m.editor.ClearSelection()
		// cleanContent unchanged — editor now differs from it, so isDirty() returns true
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		m.activePanel = editorPanel
		m.editorMode = modeEdit
		cmd := m.editor.Focus()
		return m, cmd
	case "n", "esc":
		m.aiRunOnSelection = false
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	case "tab":
		m.paneFocus = togglePaneFocus(m.paneFocus)
		return m, nil
	case "up", "k":
		m.scrollFocusedPane(false, 1)
		return m, nil
	case "down", "j":
		m.scrollFocusedPane(true, 1)
		return m, nil
	}
	return m, nil
}

func pluginDiffView(left, right viewport.Model, focus paneFocus, width, height int) string {
	originalHeader := diffHeaderOriginalStyle.Render("── Original ──")
	newHeader := diffHeaderNewStyle.Render("── New ──")
	if focus == paneFocusLeft {
		originalHeader = paneFocusedHeaderStyle.Render("── Original ──")
	} else {
		newHeader = paneFocusedHeaderStyle.Render("── New ──")
	}

	halfWidth := width / 2

	leftPanel := diffBorderStyle.Width(halfWidth).Height(height).Render(
		originalHeader + "\n" + left.View())

	rightPanel := lipgloss.NewStyle().Width(width - halfWidth - 1).Height(height).Render(
		newHeader + "\n" + right.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func newDiffViewports(original, result string, width, height int) (viewport.Model, viewport.Model) {
	halfWidth := width / 2
	contentHeight := height - 1

	leftWidth := halfWidth - 2
	left := viewport.New(leftWidth, contentHeight)
	left.SetContent(wordWrap(original, leftWidth))

	rightWidth := width - halfWidth - 3 // -1 border, -2 margin
	right := viewport.New(rightWidth, contentHeight)
	right.SetContent(wordWrap(result, rightWidth))

	return left, right
}
