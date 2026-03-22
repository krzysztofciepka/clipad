package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	statusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	statusErrorStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("196")).
		Bold(true)
)

type StatusBar struct {
	width      int
	treeActive bool
	filename   string
	line       int
	col        int
	dirty      bool
	errMsg     string
	fileOpen   bool
}

type hint struct {
	key   string
	label string
}

func (s StatusBar) View() string {
	hints := []hint{
		{"^S", "save"},
		{"^N", "new"},
	}

	if s.treeActive {
		hints = append(hints, hint{"^D", "del"}, hint{"^F", "folder"})
	}

	hints = append(hints,
		hint{"^Q", "quit"},
		hint{"Tab", "switch"},
		hint{"^P", "preview"},
	)

	if s.fileOpen {
		hints = append(hints, hint{"^Spc", "plugins"})
	}

	right := ""
	if s.errMsg != "" {
		right = statusErrorStyle.Render(s.errMsg)
	} else if s.filename != "" {
		modified := ""
		if s.dirty {
			modified = " [+]"
		}
		right = fmt.Sprintf("%d:%d  %s%s", s.line, s.col, s.filename, modified)
	}

	// Build left side, dropping hints from the end if they don't fit
	rightWidth := lipgloss.Width(right)
	left := ""
	for _, h := range hints {
		entry := statusKeyStyle.Render(h.key) + " " + h.label + "  "
		if lipgloss.Width(left)+lipgloss.Width(entry)+rightWidth+2 > s.width {
			break
		}
		left += entry
	}

	gap := s.width - lipgloss.Width(left) - rightWidth
	if gap < 1 {
		gap = 1
	}

	bar := left + strings.Repeat(" ", gap) + right
	return statusBarStyle.Width(s.width).Render(bar)
}
