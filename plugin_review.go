package main

import (
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// paneRightWidth is the inner width of the right pane in the side-by-side
// diff/review views, matching the geometry newDiffViewports uses.
func paneRightWidth(editorWidth int) int {
	w := editorWidth - editorWidth/2 - 3
	if w < 1 {
		w = 1
	}
	return w
}

// paneRightMarkdown renders the AI output as markdown for the right pane of
// the diff/review views. The AI output is shown the same way the editor's
// Ctrl+P preview renders notes; the raw text (m.pluginDiffResult) is what an
// accept actually inserts. Falls back to wrapped plain text on render error.
func paneRightMarkdown(result string, width int) string {
	rendered, err := renderMarkdown(result, width)
	if err != nil {
		return wordWrap(result, width)
	}
	return rendered
}

// paneFocus is which pane scroll/keys act on in the side-by-side diff and
// review views.
type paneFocus int

const (
	paneFocusRight paneFocus = iota // default: the AI output pane (right)
	paneFocusLeft                   // the original note pane (left)
)

// togglePaneFocus flips focus between the two side-by-side panes.
func togglePaneFocus(f paneFocus) paneFocus {
	if f == paneFocusLeft {
		return paneFocusRight
	}
	return paneFocusLeft
}

// scrollFocusedPane scrolls the currently focused side-by-side pane by n
// lines (down when down is true, otherwise up).
func (m *model) scrollFocusedPane(down bool, n int) {
	switch {
	case m.paneFocus == paneFocusLeft && down:
		m.pluginDiffViewL.LineDown(n)
	case m.paneFocus == paneFocusLeft:
		m.pluginDiffViewL.LineUp(n)
	case down:
		m.pluginDiffViewR.LineDown(n)
	default:
		m.pluginDiffViewR.LineUp(n)
	}
}

var (
	reviewHeaderNoteStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)

	reviewHeaderReviewStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("75")).
				Padding(0, 1)

	// paneFocusedHeaderStyle highlights the focused pane's header in both
	// the diff and review side-by-side views.
	paneFocusedHeaderStyle = lipgloss.NewStyle().
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
func pluginReviewView(left, right viewport.Model, focus paneFocus, width, height int) string {
	noteHeader := reviewHeaderNoteStyle.Render("── Note ──")
	reviewHeader := reviewHeaderReviewStyle.Render("── Review ──")
	if focus == paneFocusLeft {
		noteHeader = paneFocusedHeaderStyle.Render("── Note ──")
	} else {
		reviewHeader = paneFocusedHeaderStyle.Render("── Review ──")
	}

	halfWidth := width / 2

	leftPanel := reviewBorderStyle.Width(halfWidth).Height(height).Render(
		noteHeader + "\n" + left.View())

	rightPanel := lipgloss.NewStyle().Width(width - halfWidth - 1).Height(height).Render(
		reviewHeader + "\n" + right.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// handlePaneMouse scrolls the pane the cursor is over when a wheel event
// arrives during the diff or review side-by-side views. X is absolute (the
// editor area begins at m.treeWidth); the split is at the editor's
// horizontal midpoint.
func (m model) handlePaneMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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
		m.paneFocus = togglePaneFocus(m.paneFocus)
		return m, nil
	case "up", "k":
		m.scrollFocusedPane(false, 1)
		return m, nil
	case "down", "j":
		m.scrollFocusedPane(true, 1)
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
