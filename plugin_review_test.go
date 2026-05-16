package main

import (
	"strings"
	"testing"

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
