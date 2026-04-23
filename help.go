package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117")).
			PaddingBottom(1)

	helpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("252")).
				PaddingTop(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	helpHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			PaddingTop(1)
)

type helpEntry struct {
	key  string
	desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

func helpSections() []helpSection {
	return []helpSection{
		{
			title: "Global",
			entries: []helpEntry{
				{"Ctrl+S", "Save"},
				{"Ctrl+N", "New note"},
				{"Ctrl+R", "Find & replace"},
				{"Ctrl+P", "Toggle markdown preview"},
				{"Ctrl+Q", "Quit"},
				{"Tab", "Switch panels"},
				{"Ctrl+Space", "Plugin selector"},
				{"Ctrl+G", "AI shortcut selector"},
				{"Ctrl+L", "New AI shortcut"},
			},
		},
		{
			title: "Tree",
			entries: []helpEntry{
				{"Up/Down", "Navigate (previews file)"},
				{"Enter", "Open file / toggle folder"},
				{"/", "Fuzzy filter"},
				{"Ctrl+E", "Rename"},
				{"Ctrl+D", "Delete"},
				{"Ctrl+F", "Create folder"},
			},
		},
		{
			title: "Editor",
			entries: []helpEntry{
				{"Shift+Arrow", "Select text"},
				{"Ctrl+C / X / V", "Copy / cut / paste"},
				{"Ctrl+A", "Select all"},
				{"Ctrl+Z", "Undo"},
				{"Ctrl+Shift+Z", "Redo (or Ctrl+Y)"},
				{"Esc", "Return to tree"},
			},
		},
	}
}

func helpView(width, height int) string {
	var b strings.Builder
	b.WriteString(helpTitleStyle.Render("Clipad — Keybindings"))
	b.WriteString("\n")

	keyCol := 14
	for _, sec := range helpSections() {
		b.WriteString(helpSectionStyle.Render(sec.title))
		b.WriteString("\n")
		for _, e := range sec.entries {
			pad := keyCol - len([]rune(e.key))
			if pad < 1 {
				pad = 1
			}
			b.WriteString(helpKeyStyle.Render(e.key))
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(helpDescStyle.Render(e.desc))
			b.WriteString("\n")
		}
	}
	b.WriteString(helpHintStyle.Render("Press Esc to close"))

	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Padding(1, 2).
		Render(b.String())
}
