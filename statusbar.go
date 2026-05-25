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

	// Writing metrics inputs (Task 29).
	editorFocused   bool   // activePanel == editorPanel
	previewMode     bool   // editorMode == modePreview
	selectionActive bool   // editor has an active selection
	bufferText      string // full editor buffer
	selectionText   string // selected text, "" when no selection
}

type hint struct {
	key   string
	label string
}

// metricTokens returns the writing-metrics tokens to display (highest priority
// first) and an optional prefix, or ("", nil) when no metrics should show.
// Token order encodes the adaptive drop rule: drop the last token first, keep
// "W words" longest.
func (s StatusBar) metricTokens() (prefix string, tokens []string) {
	if !s.editorFocused {
		return "", nil
	}
	if s.selectionActive {
		words, chars := computeMetrics(s.selectionText)
		if words == 0 {
			return "", nil
		}
		return "sel: ", []string{
			fmt.Sprintf("%d words", words),
			fmt.Sprintf("%d chars", chars),
		}
	}
	words, chars := computeMetrics(s.bufferText)
	if words == 0 {
		return "", nil
	}
	if s.previewMode {
		return "", []string{fmt.Sprintf("~%dm read", readingMinutes(words))}
	}
	return "", []string{
		fmt.Sprintf("%d words", words),
		fmt.Sprintf("%d chars", chars),
	}
}

// fitMetrics joins tokens (highest priority first) with " · " after prefix,
// dropping the lowest-priority token until the result fits within budget.
// Returns "" if even the first token does not fit.
func fitMetrics(prefix string, tokens []string, budget int) string {
	for len(tokens) > 0 {
		s := prefix + strings.Join(tokens, " · ")
		if lipgloss.Width(s) <= budget {
			return s
		}
		tokens = tokens[:len(tokens)-1]
	}
	return ""
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

	// Writing metrics, placed between the hints and the position/file segment.
	// Sheds tokens to fit and takes priority over trailing hints (which the
	// loop below drops to make room).
	if prefix, tokens := s.metricTokens(); len(tokens) > 0 {
		sep := 0
		if right != "" {
			sep = 2
		}
		budget := contentWidth - lipgloss.Width(right) - sep
		if metrics := fitMetrics(prefix, tokens, budget); metrics != "" {
			if right == "" {
				right = metrics
			} else {
				right = metrics + "  " + right
			}
		}
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
