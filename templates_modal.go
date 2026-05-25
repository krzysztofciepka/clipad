package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// templatePickerView renders the template chooser, reusing the shortcut-picker
// styles defined in shortcuts_modal.go.
func templatePickerView(templates []string, cursor, width, height int) string {
	var rows []string
	for i, name := range templates {
		if i == cursor {
			rows = append(rows, shortcutCursorStyle.Render("> "+name))
		} else {
			rows = append(rows, shortcutItemStyle.Render("  "+name))
		}
	}
	hint := shortcutHintStyle.Render("Enter:select  ↑/↓:move  Esc:cancel")
	content := strings.Join(rows, "\n") + "\n" + hint
	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(content)
}
