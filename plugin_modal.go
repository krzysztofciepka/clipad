package main

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	pluginItemStyle = lipgloss.NewStyle().
		PaddingLeft(1)

	pluginCursorStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)
)

func pluginSelectorView(plugins []Plugin, cursor int, width int) string {
	items := ""
	for i, p := range plugins {
		name := p.Name()
		if i == cursor {
			name = pluginCursorStyle.Render("> " + name)
		} else {
			name = pluginItemStyle.Render("  " + name)
		}
		if i > 0 {
			items += "  "
		}
		items += name
	}

	return statusBarStyle.Width(width).Render(items)
}
