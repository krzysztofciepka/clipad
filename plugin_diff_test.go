package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func diffModel(t *testing.T) model {
	t.Helper()
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editorWidth = 80
	m.editorHeight = 10
	m.editor.SetValue("original note body")
	m.pluginDiffOriginal = "original note body"
	m.pluginDiffResult = "# New\n\nrewritten body line one\nline two\nline three\nline four"
	m.inputMode = inputPluginDiff
	m.paneFocus = paneFocusRight
	m.pluginActive = &fakePlugin{name: "fake"}
	m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
		m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	return m
}

func TestHandlePluginDiff_TabTogglesFocus(t *testing.T) {
	m := diffModel(t)
	n, _ := m.handlePluginDiff(tea.KeyMsg{Type: tea.KeyTab})
	nm := n.(model)
	if nm.paneFocus != paneFocusLeft {
		t.Fatalf("after Tab: paneFocus = %v, want paneFocusLeft", nm.paneFocus)
	}
	n2, _ := nm.handlePluginDiff(tea.KeyMsg{Type: tea.KeyTab})
	if n2.(model).paneFocus != paneFocusRight {
		t.Errorf("after second Tab: want paneFocusRight")
	}
}

func TestHandlePluginDiff_ScrollFocusedPane(t *testing.T) {
	m := diffModel(t)
	// Force tiny, independently scrollable viewports.
	m.pluginDiffViewL = viewport.New(20, 1)
	m.pluginDiffViewL.SetContent("a\nb\nc\nd\ne")
	m.pluginDiffViewR = viewport.New(20, 1)
	m.pluginDiffViewR.SetContent("v\nw\nx\ny\nz")

	// Default focus = right pane.
	n, _ := m.handlePluginDiff(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	nm := n.(model)
	if nm.pluginDiffViewR.YOffset <= 0 {
		t.Errorf("right pane did not scroll down while focused: offset %d", nm.pluginDiffViewR.YOffset)
	}
	if nm.pluginDiffViewL.YOffset != 0 {
		t.Errorf("left pane moved while right was focused (panes no longer independent)")
	}
	// Switch focus to the left pane and scroll it.
	nm.paneFocus = paneFocusLeft
	n2, _ := nm.handlePluginDiff(tea.KeyMsg{Type: tea.KeyDown})
	nm2 := n2.(model)
	if nm2.pluginDiffViewL.YOffset <= 0 {
		t.Errorf("left pane did not scroll when focused: offset %d", nm2.pluginDiffViewL.YOffset)
	}
}

func TestHandlePluginDiff_AcceptInsertsRawResultNotMarkdown(t *testing.T) {
	m := diffModel(t)
	// Simulate the post-stream state where the New pane shows rendered markdown.
	m.pluginDiffViewR.SetContent(paneRightMarkdown(m.pluginDiffResult, paneRightWidth(m.editorWidth)))
	n, _ := m.handlePluginDiff(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := n.(model)
	if nm.editor.Value() != "# New\n\nrewritten body line one\nline two\nline three\nline four" {
		t.Errorf("accept inserted rendered/altered text, want raw result; got %q", nm.editor.Value())
	}
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone after accept", nm.inputMode)
	}
}

func TestPluginDoneMsg_DiffNonEmpty_RendersMarkdownInNewPane(t *testing.T) {
	m := diffModel(t)
	ch := make(chan string)
	close(ch)
	m.activeChunks = ch

	next, _ := m.Update(pluginDoneMsg{chunks: ch})
	nm := next.(model)

	if nm.inputMode != inputPluginDiff {
		t.Fatalf("inputMode = %v, want inputPluginDiff (changed result stays open)", nm.inputMode)
	}
	view := nm.pluginDiffViewR.View()
	if strings.Contains(view, "# New") {
		t.Errorf("New pane still shows raw markdown syntax:\n%s", view)
	}
	if !strings.Contains(view, "New") || !strings.Contains(view, "rewritten body") {
		t.Errorf("New pane missing rendered text:\n%s", view)
	}
}

func TestPluginDoneMsg_DiffNoChange_ClosesUnchanged(t *testing.T) {
	m := diffModel(t)
	m.pluginDiffResult = m.pluginDiffOriginal // identical -> "No changes"
	ch := make(chan string)
	close(ch)
	m.activeChunks = ch
	before := m.editor.Value()

	next, _ := m.Update(pluginDoneMsg{chunks: ch})
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone for no-change diff", nm.inputMode)
	}
	if nm.editor.Value() != before {
		t.Errorf("editor changed on no-change dismissal")
	}
}

func TestPluginDiffView_HighlightsFocusedHeader(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	left, right := newDiffViewports("a", "b", 80, 10)
	leftFocused := pluginDiffView(left, right, paneFocusLeft, 80, 10)
	rightFocused := pluginDiffView(left, right, paneFocusRight, 80, 10)
	if leftFocused == rightFocused {
		t.Error("focus has no visible effect on diff view headers")
	}
	if !strings.Contains(leftFocused, "Original") || !strings.Contains(rightFocused, "New") {
		t.Error("diff view missing Original/New headers")
	}
}

func TestUpdate_DiffMode_RoutesWheelToPaneMouse(t *testing.T) {
	m := diffModel(t)
	m.treeWidth = 0
	m.pluginDiffViewR = viewport.New(20, 1)
	m.pluginDiffViewR.SetContent("v\nw\nx\ny\nz")
	next, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, X: 60, Y: 3})
	nm := next.(model)
	if nm.pluginDiffViewR.YOffset <= 0 {
		t.Errorf("wheel over right half in diff mode did not scroll New pane: offset %d",
			nm.pluginDiffViewR.YOffset)
	}
}
