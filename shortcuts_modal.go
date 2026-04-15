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
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1).
			Render(content)
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
