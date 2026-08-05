package main

import "testing"

func testRowInputs() ([]AIShortcut, []FabricPattern) {
	return []AIShortcut{
			{Name: "tldr", Description: "Add a TL;DR at the top"},
			{Name: "critique", Description: "Flag issues"},
		}, []FabricPattern{
			{Name: "summarize", Description: "Summarize content"},
			{Name: "extract_wisdom", Description: "Pull out insights"},
		}
}

func TestBuildShortcutRows_HeaderSeparatesSections(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5 (2 shortcuts + header + 2 patterns)", len(rows))
	}
	kinds := []rowKind{rowShortcut, rowShortcut, rowHeader, rowFabric, rowFabric}
	for i, want := range kinds {
		if rows[i].kind != want {
			t.Errorf("row %d kind = %v, want %v", i, rows[i].kind, want)
		}
	}
	if rows[2].name != fabricSectionTitle {
		t.Errorf("header name = %q, want %q", rows[2].name, fabricSectionTitle)
	}
	if rows[3].index != 0 || rows[4].index != 1 {
		t.Errorf("pattern indexes = [%d, %d], want [0, 1]", rows[3].index, rows[4].index)
	}
}

func TestBuildShortcutRows_NoHeaderWithoutPatterns(t *testing.T) {
	shortcuts, _ := testRowInputs()
	rows := buildShortcutRows(shortcuts, nil, "")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, r := range rows {
		if r.kind != rowShortcut {
			t.Errorf("row %d kind = %v, want rowShortcut", i, r.kind)
		}
	}
}

func TestBuildShortcutRows_FilterNarrowsBothSections(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "summ")
	// "summarize" matches; no shortcut does, so the header leads.
	if len(rows) != 2 {
		t.Fatalf("got %d rows (%v), want 2", len(rows), rows)
	}
	if rows[0].kind != rowHeader {
		t.Errorf("row 0 kind = %v, want rowHeader", rows[0].kind)
	}
	if rows[1].kind != rowFabric || rows[1].name != "summarize" {
		t.Errorf("row 1 = %+v, want the summarize pattern", rows[1])
	}
}

func TestBuildShortcutRows_FilterExcludingAllPatternsDropsHeader(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "tldr")
	if len(rows) != 1 {
		t.Fatalf("got %d rows (%v), want 1", len(rows), rows)
	}
	if rows[0].kind != rowShortcut || rows[0].name != "tldr" {
		t.Errorf("row 0 = %+v, want the tldr shortcut", rows[0])
	}
}

func TestBuildShortcutRows_FilterMatchingNothingReturnsEmpty(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	if rows := buildShortcutRows(shortcuts, patterns, "zzzzqqq"); len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestNextSelectableRow_SkipsHeader(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := nextSelectableRow(rows, 1, 1); got != 3 {
		t.Errorf("down from 1 = %d, want 3 (header at 2 skipped)", got)
	}
	if got := nextSelectableRow(rows, 3, -1); got != 1 {
		t.Errorf("up from 3 = %d, want 1 (header at 2 skipped)", got)
	}
}

func TestNextSelectableRow_StopsAtEnds(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := nextSelectableRow(rows, 0, -1); got != 0 {
		t.Errorf("up from 0 = %d, want 0", got)
	}
	if got := nextSelectableRow(rows, 4, 1); got != 4 {
		t.Errorf("down from 4 = %d, want 4", got)
	}
}

func TestClampSelectableRow_MovesOffHeader(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := clampSelectableRow(rows, 2); got != 1 {
		t.Errorf("clamp on header = %d, want 1 (prefers backwards)", got)
	}
	if got := clampSelectableRow(rows, 99); got != 4 {
		t.Errorf("clamp past end = %d, want 4", got)
	}
	// Header first: the only way off it is forwards.
	patternsOnly := buildShortcutRows(nil, patterns, "")
	if got := clampSelectableRow(patternsOnly, 0); got != 1 {
		t.Errorf("clamp on leading header = %d, want 1", got)
	}
	if got := clampSelectableRow(nil, 3); got != 0 {
		t.Errorf("clamp on empty rows = %d, want 0", got)
	}
}

func TestClampShortcutOffset_KeepsCursorVisible(t *testing.T) {
	if got := clampShortcutOffset(12, 0, 10); got != 3 {
		t.Errorf("scrolling down = %d, want 3", got)
	}
	if got := clampShortcutOffset(2, 5, 10); got != 2 {
		t.Errorf("scrolling up = %d, want 2", got)
	}
	if got := clampShortcutOffset(4, 0, 10); got != 0 {
		t.Errorf("cursor already visible = %d, want 0", got)
	}
	if got := clampShortcutOffset(4, 0, 0); got != 0 {
		t.Errorf("zero-height window = %d, want 0", got)
	}
}

func TestVisibleShortcutRows_ReservesFooter(t *testing.T) {
	if got := visibleShortcutRows(20); got != 18 {
		t.Errorf("visibleShortcutRows(20) = %d, want 18", got)
	}
	if got := visibleShortcutRows(1); got != 1 {
		t.Errorf("visibleShortcutRows(1) = %d, want 1 (never below one row)", got)
	}
}

func TestSelectedRow_OutOfRangeIsNotSelectable(t *testing.T) {
	shortcuts, patterns := testRowInputs()
	rows := buildShortcutRows(shortcuts, patterns, "")
	if got := selectedRow(rows, 0); got.kind != rowShortcut || got.index != 0 {
		t.Errorf("selectedRow(0) = %+v", got)
	}
	if got := selectedRow(rows, 99); got.kind != rowHeader {
		t.Errorf("selectedRow(99) kind = %v, want rowHeader (unselectable sentinel)", got.kind)
	}
	if got := selectedRow(nil, 0); got.kind != rowHeader {
		t.Errorf("selectedRow on empty rows kind = %v, want rowHeader", got.kind)
	}
}
