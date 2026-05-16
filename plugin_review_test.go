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
