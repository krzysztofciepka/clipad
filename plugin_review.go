package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type reviewFocus int

const (
	reviewFocusReview reviewFocus = iota // default: the AI review pane (right)
	reviewFocusNote                      // the original note pane (left)
)

var (
	reviewHeaderNoteStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)

	reviewHeaderReviewStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("75")).
				Padding(0, 1)

	reviewFocusedHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("0")).
					Background(lipgloss.Color("75")).
					Padding(0, 1)

	reviewBorderStyle = lipgloss.NewStyle().
				BorderRight(true).
				BorderStyle(lipgloss.NormalBorder())
)

// pluginReviewView renders the read-only side-by-side review: the original
// note on the left, the AI-generated review on the right. The focused pane's
// header is highlighted.
func pluginReviewView(left, right viewport.Model, focus reviewFocus, width, height int) string {
	noteHeader := reviewHeaderNoteStyle.Render("── Note ──")
	reviewHeader := reviewHeaderReviewStyle.Render("── Review ──")
	if focus == reviewFocusNote {
		noteHeader = reviewFocusedHeaderStyle.Render("── Note ──")
	} else {
		reviewHeader = reviewFocusedHeaderStyle.Render("── Review ──")
	}

	halfWidth := width / 2

	leftPanel := reviewBorderStyle.Width(halfWidth).Height(height).Render(
		noteHeader + "\n" + left.View())

	rightPanel := lipgloss.NewStyle().Width(width - halfWidth - 1).Height(height).Render(
		reviewHeader + "\n" + right.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}
