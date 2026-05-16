package main

import (
	"strings"
	"testing"

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

var _ = tea.KeyMsg{}
