package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	shortcutItemStyle = lipgloss.NewStyle().
		PaddingLeft(1)

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

	shortcutDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
)

func truncateRight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

func shortcutSelectorView(shortcuts []AIShortcut, cursor int, provider string, width, height int) string {
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

	maxName := 0
	for _, s := range shortcuts {
		if n := len([]rune(s.Name)); n > maxName {
			maxName = n
		}
	}
	nameCol := maxName + 2

	descBudget := width - 2 - 2 - nameCol - 3
	if descBudget < 0 {
		descBudget = 0
	}

	var rows []string
	for i, s := range shortcuts {
		namePart := s.Name + strings.Repeat(" ", nameCol-len([]rune(s.Name)))
		var line string
		if i == cursor {
			line = shortcutCursorStyle.Render("> " + namePart)
		} else {
			line = shortcutItemStyle.Render("  " + namePart)
		}
		if s.Description != "" {
			desc := truncateRight(s.Description, descBudget)
			if desc != "" {
				line += shortcutDescStyle.Render("— " + desc)
			}
		}
		rows = append(rows, line)
	}

	items := strings.Join(rows, "\n")
	providerLine := shortcutHintStyle.Render("Provider: " + provider + "  (p:cycle)")
	hint := shortcutHintStyle.Render("Enter:run  e:edit  d:delete  Ctrl+↑/↓:reorder  Esc:close")
	content := items + "\n" + providerLine + "\n" + hint

	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(content)
}
