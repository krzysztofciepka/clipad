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

	statusFlashStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("76"))
)

type StatusBar struct {
	width         int
	treeActive    bool
	filename      string
	line          int
	col           int
	dirty         bool
	errMsg        string
	flashMsg      string // non-error flash message (e.g. "Auto-saved")
	fileOpen      bool
	indexerStatus string // e.g. "[idx 47/312]"
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
		hints = append(hints, hint{"^Spc", "plugins"}, hint{"^G", "AI"})
	}

	right := ""
	if s.indexerStatus != "" {
		right = statusFlashStyle.Render(s.indexerStatus) + "  "
	}
	if s.errMsg != "" {
		right += statusErrorStyle.Render(s.errMsg)
	} else if s.flashMsg != "" {
		right += statusFlashStyle.Render(s.flashMsg)
	} else if s.filename != "" {
		modified := ""
		if s.dirty {
			modified = " [+]"
		}
		right += fmt.Sprintf("%d:%d  %s%s", s.line, s.col, s.filename, modified)
	}

	// Available content width (subtract padding)
	contentWidth := s.width - 2
	if contentWidth < 0 {
		contentWidth = 0
	}

	// Build left side, dropping hints from the end if they don't fit
	rightWidth := lipgloss.Width(right)
	left := ""
	for _, h := range hints {
		entry := statusKeyStyle.Render(h.key) + " " + h.label + "  "
		if lipgloss.Width(left)+lipgloss.Width(entry)+rightWidth > contentWidth {
			break
		}
		left += entry
	}

	gap := contentWidth - lipgloss.Width(left) - rightWidth
	if gap < 0 {
		gap = 0
	}

	bar := left + strings.Repeat(" ", gap) + right
	return statusBarStyle.Width(s.width).MaxWidth(s.width).Render(bar)
}
