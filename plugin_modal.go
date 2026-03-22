package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	pluginModalStyle = lipgloss.NewStyle().
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("117"))

	pluginItemStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	pluginSelectedStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	pluginDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
)

func pluginModalView(plugins []Plugin, cursor int, width, height int) string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Render("Plugins")
	b.WriteString(title)
	b.WriteString("\n\n")

	for i, p := range plugins {
		line := fmt.Sprintf("%s  %s", p.Name(), pluginDescStyle.Render(p.Description()))
		if i == cursor {
			line = pluginSelectedStyle.Render(fmt.Sprintf("> %s  %s", p.Name(), p.Description()))
		} else {
			line = pluginItemStyle.Render(line)
		}
		b.WriteString(line)
		if i < len(plugins)-1 {
			b.WriteString("\n")
		}
	}

	// Size the modal to fit content, not the full panel
	modalWidth := width * 2 / 3
	if modalWidth < 40 {
		modalWidth = 40
	}
	modal := pluginModalStyle.Width(modalWidth).Render(b.String())

	// Center the modal over the panel area
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}
