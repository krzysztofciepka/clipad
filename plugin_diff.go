package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	diffHeaderOriginal = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Padding(0, 1).
		Render("── Original ──")

	diffHeaderNew = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("76")).
		Padding(0, 1).
		Render("── New ──")

	diffBorderStyle = lipgloss.NewStyle().
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder())
)

func (m model) handlePluginDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.editor.SetValue(m.pluginDiffResult)
		// cleanContent unchanged — editor now differs from it, so isDirty() returns true
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
	case "n", "esc":
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
	case "ctrl+q", "ctrl+c":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	case "up", "k":
		m.pluginDiffViewL.LineUp(1)
		m.pluginDiffViewR.LineUp(1)
		return m, nil
	case "down", "j":
		m.pluginDiffViewL.LineDown(1)
		m.pluginDiffViewR.LineDown(1)
		return m, nil
	}
	return m, nil
}

func pluginDiffView(left, right viewport.Model, width, height int) string {
	halfWidth := width / 2

	leftPanel := diffBorderStyle.Width(halfWidth).Height(height).Render(
		diffHeaderOriginal + "\n" + left.View())

	rightPanel := lipgloss.NewStyle().Width(width - halfWidth).Height(height).Render(
		diffHeaderNew + "\n" + right.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func newDiffViewports(original, result string, width, height int) (viewport.Model, viewport.Model) {
	halfWidth := width / 2
	contentHeight := height - 1

	left := viewport.New(halfWidth-2, contentHeight)
	left.SetContent(original)

	right := viewport.New(width-halfWidth-2, contentHeight)
	right.SetContent(result)

	return left, right
}
