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

func (s StatusBar) View() string {
	left := statusKeyStyle.Render("^S") + " save  " +
		statusKeyStyle.Render("^N") + " new  "

	if s.treeActive {
		left += statusKeyStyle.Render("^D") + " del  " +
			statusKeyStyle.Render("^F") + " folder  "
	}

	left += statusKeyStyle.Render("^Q") + " quit  " +
		statusKeyStyle.Render("Tab") + " switch  " +
		statusKeyStyle.Render("^P") + " preview"

	if s.fileOpen {
		left += "  " + statusKeyStyle.Render("^Space") + " plugins"
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

	gap := s.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	bar := left + strings.Repeat(" ", gap) + right
	return statusBarStyle.Width(s.width).Render(bar)
}
