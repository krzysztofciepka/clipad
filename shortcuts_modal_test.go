package main

import (
	"strings"
	"testing"
)

func TestShortcutSelectorView_ShowsDescriptions(t *testing.T) {
	rows := buildShortcutRows([]AIShortcut{
		{Name: "prd", Description: "Turn text into a PRD with TBDs for gaps"},
		{Name: "tldr", Description: "Add a TL;DR at the top"},
	}, nil, "")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20)
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
	rows := buildShortcutRows([]AIShortcut{
		{Name: "a", Description: "first"},
		{Name: "longname", Description: "second"},
	}, nil, "")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20)
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
	rows := buildShortcutRows([]AIShortcut{
		{Name: "bare", Description: ""},
		{Name: "full", Description: "has a description"},
	}, nil, "")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20)
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
	rows := buildShortcutRows([]AIShortcut{
		{Name: "a", Description: longDesc},
	}, nil, "")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 30, 20)
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
	out := shortcutSelectorView(nil, 0, 0, "openrouter", false, 80, 10)
	if !strings.Contains(out, "No shortcuts") {
		t.Errorf("empty-list rendering changed: %q", out)
	}
}

func TestShortcutSelectorView_RendersFabricSection(t *testing.T) {
	rows := buildShortcutRows(
		[]AIShortcut{{Name: "tldr", Description: "Add a TL;DR"}},
		[]FabricPattern{{Name: "summarize", Description: "Summarize content"}},
		"")
	out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20)
	if !strings.Contains(out, fabricSectionTitle) {
		t.Error("missing fabric section header")
	}
	if !strings.Contains(out, "summarize") {
		t.Error("missing fabric pattern name")
	}
	if !strings.Contains(out, "Summarize content") {
		t.Error("missing fabric pattern description")
	}
}

func TestShortcutSelectorView_ScrollsToOffset(t *testing.T) {
	var shortcuts []AIShortcut
	for _, n := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		shortcuts = append(shortcuts, AIShortcut{Name: n})
	}
	rows := buildShortcutRows(shortcuts, nil, "")
	// height 5 leaves 3 visible rows after the two footer lines.
	out := shortcutSelectorView(rows, 4, 2, "openrouter", false, 120, 5)
	if strings.Contains(out, "alpha") || strings.Contains(out, "bravo") {
		t.Error("rows above the offset are still rendered")
	}
	if !strings.Contains(out, "echo") {
		t.Error("cursor row 'echo' is not rendered")
	}
	if strings.Contains(out, "foxtrot") {
		t.Error("row past the window is rendered")
	}
}

func TestShortcutSelectorView_FilteringSwapsHint(t *testing.T) {
	rows := buildShortcutRows([]AIShortcut{{Name: "tldr"}}, nil, "")
	if out := shortcutSelectorView(rows, 0, 0, "openrouter", false, 120, 20); !strings.Contains(out, "/:filter") {
		t.Error("normal hint should advertise the filter key")
	}
	if out := shortcutSelectorView(rows, 0, 0, "openrouter", true, 120, 20); !strings.Contains(out, "Esc:clear") {
		t.Error("filter hint should advertise clearing the filter")
	}
}
