package main

import (
	"strings"
	"testing"
)

func TestShortcutSelectorView_ShowsDescriptions(t *testing.T) {
	shortcuts := []AIShortcut{
		{Name: "prd", Description: "Turn text into a PRD with TBDs for gaps"},
		{Name: "tldr", Description: "Add a TL;DR at the top"},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 120, 20)
	if !strings.Contains(out, "prd") {
		t.Error("missing shortcut name 'prd'")
	}
	if !strings.Contains(out, "Turn text into a PRD with TBDs for gaps") {
		t.Error("missing description text for 'prd'")
	}
	if !strings.Contains(out, "Add a TL;DR at the top") {
		t.Error("missing description text for 'tldr'")
	}
	if !strings.Contains(out, "—") {
		t.Error("missing em-dash separator between name and description")
	}
}

func TestShortcutSelectorView_NamesAlignToLongest(t *testing.T) {
	shortcuts := []AIShortcut{
		{Name: "a", Description: "first"},
		{Name: "longname", Description: "second"},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 120, 20)
	lines := strings.Split(out, "\n")
	var dashCols []int
	for _, ln := range lines {
		if idx := strings.Index(ln, "—"); idx >= 0 {
			dashCols = append(dashCols, idx)
		}
	}
	if len(dashCols) < 2 {
		t.Fatalf("expected at least 2 em-dash lines, got %d in:\n%s", len(dashCols), out)
	}
	for i := 1; i < len(dashCols); i++ {
		if dashCols[i] != dashCols[0] {
			t.Errorf("em-dash columns not aligned: %v", dashCols)
		}
	}
}

func TestShortcutSelectorView_EmptyDescriptionFallsBackToNameOnly(t *testing.T) {
	shortcuts := []AIShortcut{
		{Name: "bare", Description: ""},
		{Name: "full", Description: "has a description"},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 120, 20)
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "bare") && strings.Contains(ln, "—") {
			t.Errorf("empty-description row should not have em-dash: %q", ln)
		}
	}
	foundFull := false
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "full") && strings.Contains(ln, "—") {
			foundFull = true
		}
	}
	if !foundFull {
		t.Error("row with non-empty description is missing em-dash")
	}
}

func TestShortcutSelectorView_TruncatesLongDescription(t *testing.T) {
	longDesc := strings.Repeat("x", 500)
	shortcuts := []AIShortcut{
		{Name: "a", Description: longDesc},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 30, 20)
	if !strings.Contains(out, "…") {
		t.Error("expected ellipsis indicating truncation")
	}
	for _, ln := range strings.Split(out, "\n") {
		if len(ln) > 200 {
			t.Errorf("line appears untruncated at narrow width (len=%d): %q", len(ln), ln)
		}
	}
}

func TestShortcutSelectorView_EmptyListUnchanged(t *testing.T) {
	out := shortcutSelectorView(nil, 0, "blackbox", 80, 10)
	if !strings.Contains(out, "No shortcuts") {
		t.Errorf("empty-list rendering changed: %q", out)
	}
}
