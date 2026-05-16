package main

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestPluginReviewView_ShowsNoteAndReviewHeaders(t *testing.T) {
	left, right := newDiffViewports("the original note", "the AI review", 80, 10)
	out := pluginReviewView(left, right, reviewFocusReview, 80, 10)
	if !strings.Contains(out, "Note") {
		t.Errorf("review view missing 'Note' header:\n%s", out)
	}
	if !strings.Contains(out, "Review") {
		t.Errorf("review view missing 'Review' header:\n%s", out)
	}
	if strings.Contains(out, "Original") || strings.Contains(out, "New") {
		t.Errorf("review view must not reuse diff headers Original/New:\n%s", out)
	}
}

func TestPluginReviewView_FocusIsRenderable(t *testing.T) {
	// Force TrueColor so lipgloss emits ANSI sequences in the non-TTY test env,
	// and restore the default (no-color) profile when the test exits.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	left, right := newDiffViewports("a", "b", 80, 10)
	// Both focus states must render without panicking and produce output.
	a := pluginReviewView(left, right, reviewFocusNote, 80, 10)
	b := pluginReviewView(left, right, reviewFocusReview, 80, 10)
	if a == "" {
		t.Error("reviewFocusNote produced empty view")
	}
	if b == "" {
		t.Error("reviewFocusReview produced empty view")
	}
	if a == b {
		t.Error("focus has no visible effect on rendering")
	}
}

func runShortcutSelectWithType(t *testing.T, sc AIShortcut) model {
	t.Helper()
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	provider := defaultAIShortcutProvider
	plugin := pluginByName(m.plugins, provider)
	if plugin == nil {
		plugin = &fakePlugin{name: provider}
		m.plugins = []Plugin{plugin}
	}
	if err := savePluginConfig(provider, map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.shortcuts = []AIShortcut{sc}
	m.shortcutCursor = 0
	m.activeShortcutProvider = provider
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(pressEnter())
	return next.(model)
}

func TestShortcutSelect_ReviewType_EntersReviewMode(t *testing.T) {
	nm := runShortcutSelectWithType(t, AIShortcut{Name: "critique", Description: "d", Prompt: "p", Type: "review"})
	if nm.inputMode != inputPluginReview {
		t.Fatalf("inputMode = %v, want inputPluginReview", nm.inputMode)
	}
	if nm.reviewFocus != reviewFocusReview {
		t.Errorf("reviewFocus = %v, want reviewFocusReview (default)", nm.reviewFocus)
	}
}

func TestShortcutSelect_ReplaceType_EntersDiffMode(t *testing.T) {
	nm := runShortcutSelectWithType(t, AIShortcut{Name: "tighten", Description: "d", Prompt: "p", Type: "replace"})
	if nm.inputMode != inputPluginDiff {
		t.Fatalf("inputMode = %v, want inputPluginDiff", nm.inputMode)
	}
}

func TestShortcutSelect_InferredReview_EntersReviewMode(t *testing.T) {
	// No explicit Type -> resolved by name (critique => review).
	nm := runShortcutSelectWithType(t, AIShortcut{Name: "critique", Description: "d", Prompt: "p"})
	if nm.inputMode != inputPluginReview {
		t.Fatalf("inputMode = %v, want inputPluginReview", nm.inputMode)
	}
}

func reviewModel(t *testing.T) model {
	t.Helper()
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("the untouched note")
	m.pluginDiffOriginal = "the untouched note"
	m.pluginDiffResult = "line one\nline two\nline three\nline four\nline five"
	m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
		m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	m.inputMode = inputPluginReview
	m.reviewFocus = reviewFocusReview
	m.pluginActive = &fakePlugin{name: "fake"}
	return m
}

func TestHandlePluginReview_TabTogglesFocus(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyTab})
	nm := next.(model)
	if nm.reviewFocus != reviewFocusNote {
		t.Fatalf("after Tab: reviewFocus = %v, want reviewFocusNote", nm.reviewFocus)
	}
	next2, _ := nm.handlePluginReview(tea.KeyMsg{Type: tea.KeyTab})
	if next2.(model).reviewFocus != reviewFocusReview {
		t.Errorf("after second Tab: want reviewFocusReview")
	}
}

func TestHandlePluginReview_EscClosesWithoutChangingNote(t *testing.T) {
	m := reviewModel(t)
	before := m.editor.Value()
	next, _ := m.handlePluginReview(pressEsc())
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.editor.Value() != before {
		t.Errorf("editor value changed: got %q, want %q", nm.editor.Value(), before)
	}
	if nm.pluginDiffResult != "" || nm.pluginActive != nil {
		t.Errorf("plugin state not cleared on close")
	}
}

func TestHandlePluginReview_QClosesWithoutChangingNote(t *testing.T) {
	m := reviewModel(t)
	before := m.editor.Value()
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.editor.Value() != before {
		t.Errorf("editor value changed on q-close")
	}
}

func TestHandlePluginReview_CopyWritesClipboard(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	nm := next.(model)
	got, err := clipboardReadAllForTest()
	if err != nil {
		t.Skipf("clipboard unavailable in this environment: %v", err)
	}
	if got != m.pluginDiffResult {
		t.Errorf("clipboard = %q, want %q", got, m.pluginDiffResult)
	}
	if nm.inputMode != inputPluginReview {
		t.Errorf("copy should not close review; inputMode = %v", nm.inputMode)
	}
}

func TestHandlePluginReview_ScrollFocusedPane(t *testing.T) {
	m := reviewModel(t)
	// Force tiny viewport so content is scrollable.
	m.pluginDiffViewR = viewport.New(20, 1)
	m.pluginDiffViewR.SetContent("a\nb\nc\nd\ne")
	startOffset := m.pluginDiffViewR.YOffset
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	nm := next.(model)
	if nm.pluginDiffViewR.YOffset <= startOffset {
		t.Errorf("review pane did not scroll down: offset %d -> %d", startOffset, nm.pluginDiffViewR.YOffset)
	}
}

func TestPluginDoneMsg_ReviewEmpty_Closes(t *testing.T) {
	m := reviewModel(t)
	m.pluginDiffResult = ""
	ch := make(chan string)
	close(ch)
	m.activeChunks = ch
	next, _ := m.Update(pluginDoneMsg{chunks: ch})
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.errMsg != "No review generated" {
		t.Errorf("errMsg = %q, want %q", nm.errMsg, "No review generated")
	}
	if nm.pluginActive != nil || nm.pluginDiffOriginal != "" || nm.pluginDiffResult != "" {
		t.Errorf("plugin state not cleared after empty review done")
	}
}

func TestPluginDoneMsg_ReviewNonEmpty_DoesNotClose(t *testing.T) {
	m := reviewModel(t)
	// pluginDiffResult equals pluginDiffOriginal — in diff mode this would
	// trigger "No changes" dismissal, but review mode must NOT dismiss.
	m.pluginDiffResult = m.pluginDiffOriginal
	ch := make(chan string)
	close(ch)
	m.activeChunks = ch
	next, _ := m.Update(pluginDoneMsg{chunks: ch})
	nm := next.(model)
	if nm.inputMode != inputPluginReview {
		t.Errorf("inputMode = %v, want inputPluginReview (review must not be auto-closed)", nm.inputMode)
	}
	if nm.pluginDiffResult != m.pluginDiffOriginal {
		t.Errorf("pluginDiffResult was cleared; want it preserved")
	}
}

func clipboardReadAllForTest() (string, error) {
	return clipboard.ReadAll()
}

func TestHandleReviewMouse_WheelScrollsHoveredPane(t *testing.T) {
	m := reviewModel(t)
	m.treeWidth = 0 // editor area starts at x=0; mid = editorWidth/2 = 40
	m.editorWidth = 80
	m.pluginDiffViewL = viewport.New(20, 1)
	m.pluginDiffViewL.SetContent("a\nb\nc\nd\ne")
	m.pluginDiffViewR = viewport.New(20, 1)
	m.pluginDiffViewR.SetContent("v\nw\nx\ny\nz")

	// Wheel down over the left half (x=5) scrolls the note pane.
	leftStart := m.pluginDiffViewL.YOffset
	m2, _ := m.handleReviewMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, X: 5, Y: 3})
	nm := m2.(model)
	if nm.pluginDiffViewL.YOffset <= leftStart {
		t.Errorf("left pane did not scroll on wheel over left half")
	}

	// Wheel down over the right half (x=60) scrolls the review pane.
	rightStart := nm.pluginDiffViewR.YOffset
	m3, _ := nm.handleReviewMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, X: 60, Y: 3})
	nm2 := m3.(model)
	if nm2.pluginDiffViewR.YOffset <= rightStart {
		t.Errorf("right pane did not scroll on wheel over right half")
	}

	// WheelUp over the right half scrolls the review pane back up.
	m4, _ := nm2.handleReviewMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, X: 60, Y: 3})
	nm3 := m4.(model)
	if nm3.pluginDiffViewR.YOffset >= nm2.pluginDiffViewR.YOffset {
		t.Errorf("WheelUp did not scroll right pane up: offset %d -> %d",
			nm2.pluginDiffViewR.YOffset, nm3.pluginDiffViewR.YOffset)
	}

	// Non-wheel button (Left click) is a no-op: model unchanged.
	beforeL := nm3.pluginDiffViewL.YOffset
	beforeR := nm3.pluginDiffViewR.YOffset
	beforeMode := nm3.inputMode
	m5, _ := nm3.handleReviewMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, X: 5, Y: 3})
	nm4 := m5.(model)
	if nm4.pluginDiffViewL.YOffset != beforeL || nm4.pluginDiffViewR.YOffset != beforeR {
		t.Errorf("Left click changed YOffsets: L %d->%d, R %d->%d",
			beforeL, nm4.pluginDiffViewL.YOffset, beforeR, nm4.pluginDiffViewR.YOffset)
	}
	if nm4.inputMode != beforeMode {
		t.Errorf("Left click changed inputMode: %v -> %v", beforeMode, nm4.inputMode)
	}
}

func TestReviewRightContent_RendersMarkdown(t *testing.T) {
	in := "# Heading\n\nSome *body* text here."
	got := reviewRightContent(in, 40)
	if got == "" {
		t.Fatal("reviewRightContent returned empty")
	}
	if strings.Contains(got, "# Heading") {
		t.Errorf("markdown syntax not consumed (still raw):\n%s", got)
	}
	if !strings.Contains(got, "Heading") || !strings.Contains(got, "body") {
		t.Errorf("rendered output missing source text:\n%s", got)
	}
	if got == wordWrap(in, 40) {
		t.Errorf("reviewRightContent did not render markdown (equals raw wordWrap)")
	}
}

func TestPluginDoneMsg_ReviewNonEmpty_RendersMarkdownInPane(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editorWidth = 80
	m.editorHeight = 10
	m.pluginDiffOriginal = "the note"
	m.pluginDiffResult = "# Review\n\nsome details"
	m.inputMode = inputPluginReview
	m.reviewFocus = reviewFocusReview
	m.pluginActive = &fakePlugin{name: "fake"}
	ch := make(chan string)
	close(ch)
	m.activeChunks = ch
	m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
		m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)

	next, _ := m.Update(pluginDoneMsg{chunks: ch})
	nm := next.(model)

	if nm.inputMode != inputPluginReview {
		t.Fatalf("inputMode = %v, want inputPluginReview (non-empty review stays open)", nm.inputMode)
	}
	view := nm.pluginDiffViewR.View()
	if strings.Contains(view, "# Review") {
		t.Errorf("review pane still shows raw markdown syntax:\n%s", view)
	}
	if !strings.Contains(view, "Review") || !strings.Contains(view, "details") {
		t.Errorf("review pane missing rendered text:\n%s", view)
	}
}
