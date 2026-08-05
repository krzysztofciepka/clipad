package main

import (
	"github.com/sahilm/fuzzy"
)

type rowKind int

const (
	rowShortcut rowKind = iota
	rowHeader
	rowFabric
)

// fabricSectionTitle labels the read-only fabric patterns below the user's own
// shortcuts.
const fabricSectionTitle = "── Fabric patterns ──"

// shortcutRow is one line of the Ctrl+G picker. The picker mixes editable
// shortcuts with read-only fabric patterns under a non-selectable header, so
// the cursor indexes rows rather than either underlying slice.
type shortcutRow struct {
	kind        rowKind
	index       int // into shortcuts (rowShortcut) or patterns (rowFabric); -1 for rowHeader
	name        string
	description string
}

// nameSource adapts a name slice to the fuzzy matcher, mirroring fileSource in
// filter.go.
type nameSource []string

func (n nameSource) String(i int) string { return n[i] }
func (n nameSource) Len() int            { return len(n) }

// matchIndexes returns the indexes of names matching filter, in fuzzy-rank
// order. An empty filter keeps everything in its original order.
func matchIndexes(names []string, filter string) []int {
	if filter == "" {
		idx := make([]int, len(names))
		for i := range names {
			idx[i] = i
		}
		return idx
	}
	matches := fuzzy.FindFrom(filter, nameSource(names))
	idx := make([]int, len(matches))
	for i, m := range matches {
		idx[i] = m.Index
	}
	return idx
}

// buildShortcutRows flattens the user's shortcuts and the installed fabric
// patterns into picker rows. The section header is emitted only when a pattern
// row follows it, so a vault without fabric installed — or a filter that
// excludes every pattern — leaves the picker looking exactly as it did before
// patterns existed.
func buildShortcutRows(shortcuts []AIShortcut, patterns []FabricPattern, filter string) []shortcutRow {
	names := make([]string, len(shortcuts))
	for i, s := range shortcuts {
		names[i] = s.Name
	}
	var rows []shortcutRow
	for _, i := range matchIndexes(names, filter) {
		rows = append(rows, shortcutRow{
			kind:        rowShortcut,
			index:       i,
			name:        shortcuts[i].Name,
			description: shortcuts[i].Description,
		})
	}

	patternNames := make([]string, len(patterns))
	for i, p := range patterns {
		patternNames[i] = p.Name
	}
	matched := matchIndexes(patternNames, filter)
	if len(matched) == 0 {
		return rows
	}
	rows = append(rows, shortcutRow{kind: rowHeader, index: -1, name: fabricSectionTitle})
	for _, i := range matched {
		rows = append(rows, shortcutRow{
			kind:        rowFabric,
			index:       i,
			name:        patterns[i].Name,
			description: patterns[i].Description,
		})
	}
	return rows
}

// nextSelectableRow moves from cursor by delta (+1 or -1), skipping header
// rows. Returns cursor unchanged when no selectable row lies that way.
func nextSelectableRow(rows []shortcutRow, cursor, delta int) int {
	for i := cursor + delta; i >= 0 && i < len(rows); i += delta {
		if rows[i].kind != rowHeader {
			return i
		}
	}
	return cursor
}

// clampSelectableRow pulls cursor into range and off any header row,
// preferring the row above. Returns 0 when nothing is selectable.
func clampSelectableRow(rows []shortcutRow, cursor int) int {
	if len(rows) == 0 {
		return 0
	}
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if rows[cursor].kind != rowHeader {
		return cursor
	}
	if i := nextSelectableRow(rows, cursor, -1); rows[i].kind != rowHeader {
		return i
	}
	if i := nextSelectableRow(rows, cursor, 1); rows[i].kind != rowHeader {
		return i
	}
	return cursor
}

// clampShortcutOffset scrolls the visible window so cursor stays inside it,
// mirroring how filterOffset tracks filterCursor for the file tree.
func clampShortcutOffset(cursor, offset, visible int) int {
	if visible <= 0 || offset < 0 {
		return 0
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return offset
}

// visibleShortcutRows is how many rows fit above the picker footer (the
// provider line and the hint line).
func visibleShortcutRows(height int) int {
	n := height - 2
	if n < 1 {
		n = 1
	}
	return n
}

// selectedRow returns the row under the cursor. An out-of-range cursor yields
// an unselectable header sentinel so callers can treat it like the section
// header without a separate bounds check.
func selectedRow(rows []shortcutRow, cursor int) shortcutRow {
	if cursor < 0 || cursor >= len(rows) {
		return shortcutRow{kind: rowHeader, index: -1}
	}
	return rows[cursor]
}

// shortcutRows is the picker's current row list: the user's shortcuts and the
// installed fabric patterns, narrowed by the active filter.
func (m model) shortcutRows() []shortcutRow {
	return buildShortcutRows(m.shortcuts, m.fabricPatterns, m.shortcutFilterInput.Value())
}

// selectedShortcutIndex is the index into m.shortcuts under the cursor, or -1
// when the cursor sits on the fabric header or a pattern row.
func (m model) selectedShortcutIndex() int {
	row := selectedRow(m.shortcutRows(), m.shortcutCursor)
	if row.kind != rowShortcut {
		return -1
	}
	return row.index
}
