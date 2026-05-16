package main

import (
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// reviewRightWidth is the inner width of the review (right) pane, matching
// the geometry newDiffViewports uses for the right viewport.
func reviewRightWidth(editorWidth int) int {
	w := editorWidth - editorWidth/2 - 3
	if w < 1 {
		w = 1
	}
	return w
}

// reviewRightContent renders the AI review as markdown for the read-only
// review pane (the review text is never edited, so it is shown the same way
// the editor's Ctrl+P preview renders notes). Falls back to wrapped plain
// text if markdown rendering fails.
func reviewRightContent(result string, width int) string {
	rendered, err := renderMarkdown(result, width)
	if err != nil {
		return wordWrap(result, width)
	}
	return rendered
}

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

// handleReviewMouse scrolls the pane the cursor is over when a wheel event
// arrives during review mode. X is absolute (the editor area begins at
// m.treeWidth); the split is at the editor's horizontal midpoint.
func (m model) handleReviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}
	localX := msg.X - m.treeWidth
	if m.treeWidth > 0 {
		localX-- // account for the tree's right-border column, matching hitTestPanel
	}
	overLeft := localX < m.editorWidth/2
	const lines = 3
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if overLeft {
			m.pluginDiffViewL.LineUp(lines)
		} else {
			m.pluginDiffViewR.LineUp(lines)
		}
	case tea.MouseButtonWheelDown:
		if overLeft {
			m.pluginDiffViewL.LineDown(lines)
		} else {
			m.pluginDiffViewR.LineDown(lines)
		}
	}
	return m, nil
}

func (m model) handlePluginReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if m.reviewFocus == reviewFocusReview {
			m.reviewFocus = reviewFocusNote
		} else {
			m.reviewFocus = reviewFocusReview
		}
		return m, nil
	case "up", "k":
		if m.reviewFocus == reviewFocusNote {
			m.pluginDiffViewL.LineUp(1)
		} else {
			m.pluginDiffViewR.LineUp(1)
		}
		return m, nil
	case "down", "j":
		if m.reviewFocus == reviewFocusNote {
			m.pluginDiffViewL.LineDown(1)
		} else {
			m.pluginDiffViewR.LineDown(1)
		}
		return m, nil
	case "c":
		_ = clipboard.WriteAll(m.pluginDiffResult)
		m.errMsg = "Review copied"
		return m, nil
	case "esc", "q":
		m.closePluginRun("")
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}
