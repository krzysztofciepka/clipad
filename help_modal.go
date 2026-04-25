package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	helpHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117")).
			Bold(true).
			PaddingLeft(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("215")).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	helpHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			PaddingLeft(1)
)

type helpEntry struct {
	key  string
	desc string
}

type helpSection struct {
	title   string
	entries []helpEntry
}

var helpSections = []helpSection{
	{
		title: "Global",
		entries: []helpEntry{
			{"Ctrl+S", "Save"},
			{"Ctrl+N", "New note"},
			{"Ctrl+R", "Find & replace"},
			{"Ctrl+P", "Toggle markdown preview"},
			{"Ctrl+B", "Toggle file tree"},
			{"Ctrl+Q", "Quit"},
			{"Tab", "Switch panel"},
			{"Ctrl+C / Ctrl+X / Ctrl+V", "Copy / cut / paste"},
			{"Ctrl+Space", "Plugin selector"},
			{"Ctrl+G", "AI shortcut selector"},
			{"Ctrl+L", "Create AI shortcut"},
			{"Ctrl+?", "Show this help"},
		},
	},
	{
		title: "File Tree",
		entries: []helpEntry{
			{"Up / Down", "Navigate (previews file)"},
			{"Enter", "Open file / toggle folder / Add note"},
			{"Right", "Open file in editor"},
			{"/", "Fuzzy filter"},
			{"Ctrl+E", "Rename"},
			{"Ctrl+D", "Delete"},
			{"Ctrl+F", "New folder"},
		},
	},
	{
		title: "Editor",
		entries: []helpEntry{
			{"Esc", "Return to file tree"},
		},
	},
	{
		title: "Shortcut Picker",
		entries: []helpEntry{
			{"Enter", "Run shortcut"},
			{"e", "Edit shortcut"},
			{"d", "Delete shortcut"},
			{"Ctrl+↑ / Ctrl+↓", "Reorder shortcut"},
			{"p", "Cycle AI provider"},
			{"Esc", "Close"},
		},
	},
	{
		title: "Diff View",
		entries: []helpEntry{
			{"y", "Accept changes"},
			{"n", "Reject changes"},
		},
	},
}

func helpContent(width int) string {
	keyWidth := 0
	for _, sec := range helpSections {
		for _, e := range sec.entries {
			if len(e.key) > keyWidth {
				keyWidth = len(e.key)
			}
		}
	}

	var b strings.Builder
	for i, sec := range helpSections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(helpHeaderStyle.Render(sec.title))
		b.WriteString("\n")
		for _, e := range sec.entries {
			key := e.key + strings.Repeat(" ", keyWidth-len(e.key))
			b.WriteString("  ")
			b.WriteString(helpKeyStyle.Render(key))
			b.WriteString("  ")
			b.WriteString(helpDescStyle.Render(e.desc))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpHintStyle.Render("Esc to close · ↑/↓/PgUp/PgDn to scroll"))

	return b.String()
}
