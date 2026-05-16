package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// shortcutTypeOptions is the ordered list of selectable types.
var shortcutTypeOptions = []string{"replace", "review"}

// shortcutTypeIndex returns the cursor index for a type string, defaulting
// to 0 ("replace") for anything unrecognised.
func shortcutTypeIndex(t string) int {
	for i, o := range shortcutTypeOptions {
		if o == t {
			return i
		}
	}
	return 0
}

func (m model) handleShortcutType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.shortcutTypeCursor > 0 {
			m.shortcutTypeCursor--
		}
		return m, nil
	case "down", "j":
		if m.shortcutTypeCursor < len(shortcutTypeOptions)-1 {
			m.shortcutTypeCursor++
		}
		return m, nil
	case "r":
		m.shortcutTypeCursor = shortcutTypeIndex("replace")
		return m, nil
	case "v":
		m.shortcutTypeCursor = shortcutTypeIndex("review")
		return m, nil
	case "enter":
		shortcut := AIShortcut{
			Name:        m.shortcutTempName,
			Description: m.shortcutTempDescription,
			Prompt:      m.shortcutTempPrompt,
			Type:        shortcutTypeOptions[m.shortcutTypeCursor],
		}
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
		return m, nil
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
		return m, nil
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

var (
	shortcutTypeTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	shortcutTypeCursorStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("117")).
				Bold(true)
	shortcutTypeItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	shortcutTypeHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

var shortcutTypeHelp = map[string]string{
	"replace": "Replace the note/selection with the AI output (diff + accept)",
	"review":  "Show the AI output side-by-side, read-only (note unchanged)",
}

func shortcutTypeSelectorView(cursor int, width, height int) string {
	var b strings.Builder
	b.WriteString(shortcutTypeTitleStyle.Render("Shortcut type") + "\n\n")
	for i, o := range shortcutTypeOptions {
		if i == cursor {
			b.WriteString(shortcutTypeCursorStyle.Render("> "+o) +
				shortcutTypeItemStyle.Render("  — "+shortcutTypeHelp[o]) + "\n")
		} else {
			b.WriteString(shortcutTypeItemStyle.Render("  "+o+"  — "+shortcutTypeHelp[o]) + "\n")
		}
	}
	b.WriteString("\n" + shortcutTypeHintStyle.Render("↑/↓ or r/v select  Enter:save  Esc:cancel"))
	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(b.String())
}
