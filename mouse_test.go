package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHitTestPanel(t *testing.T) {
	tests := []struct {
		name                           string
		treeWidth, width, height, x, y int
		wantHit                        panel
		wantLocalX, wantLocalY         int
		wantOK                         bool
	}{
		{"tree area", 20, 100, 30, 5, 5, treePanel, 5, 5, true},
		{"border column rejected", 20, 100, 30, 20, 5, 0, 0, 0, false},
		{"editor area", 20, 100, 30, 25, 5, editorPanel, 4, 5, true},
		{"status bar row rejected", 20, 100, 30, 5, 29, 0, 0, 0, false},
		{"out of bounds negative", 20, 100, 30, -1, 5, 0, 0, 0, false},
		{"out of bounds right", 20, 100, 30, 100, 5, 0, 0, 0, false},
		{"out of bounds below", 20, 100, 30, 5, 30, 0, 0, 0, false},
		{"narrow terminal treats all as editor", 0, 20, 30, 5, 5, editorPanel, 5, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, lx, ly, ok := hitTestPanel(tt.treeWidth, tt.width, tt.height, tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if hit != tt.wantHit || lx != tt.wantLocalX || ly != tt.wantLocalY {
				t.Errorf("hit=%v local=(%d,%d), want %v (%d,%d)",
					hit, lx, ly, tt.wantHit, tt.wantLocalX, tt.wantLocalY)
			}
		})
	}
}

func TestMousePosToEditorCursor(t *testing.T) {
	content := "hello\nworld\nfoo bar"
	// wrapWidth big enough that no line wraps
	tests := []struct {
		name                                                 string
		visualYOffset, localX, localY, numWidth, wrapWidth   int
		wantLine, wantCol                                    int
	}{
		{"first line first char", 0, 4, 0, 2, 80, 0, 0},
		{"first line middle", 0, 6, 0, 2, 80, 0, 2},
		{"past line length clamps", 0, 20, 0, 2, 80, 0, 5},
		{"second line", 0, 4, 1, 2, 80, 1, 0},
		{"visualYOffset shifts", 1, 4, 0, 2, 80, 1, 0},
		{"past content clamps to last line", 0, 4, 99, 2, 80, 2, 0},
		{"click in line number column", 0, 2, 0, 2, 80, 0, 0},
		{"click in padding", 0, 0, 0, 2, 80, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := mousePosToEditorCursor(content, tt.visualYOffset, tt.localX, tt.localY, tt.numWidth, tt.wrapWidth)
			if line != tt.wantLine || col != tt.wantCol {
				t.Errorf("got (%d,%d), want (%d,%d)", line, col, tt.wantLine, tt.wantCol)
			}
		})
	}
}

func TestMousePosToEditorCursor_Wrapping(t *testing.T) {
	// Three lines. Each line is 25 chars. Wrap at 10 chars.
	// Line 0: 25 chars → 3 visual rows (rows 0,1,2)
	// Line 1: 25 chars → 3 visual rows (rows 3,4,5)
	// Line 2: 5 chars  → 1 visual row  (row 6)
	line0 := "aaaaaaaaaaaaaaaaaaaaaaaaa"
	line1 := "bbbbbbbbbbbbbbbbbbbbbbbbb"
	line2 := "ccccc"
	content := line0 + "\n" + line1 + "\n" + line2

	tests := []struct {
		name                                                 string
		visualYOffset, localX, localY, numWidth, wrapWidth   int
		wantLine, wantCol                                    int
	}{
		{"click on visual row 0 → line 0 wrap 0", 0, 4, 0, 2, 10, 0, 0},
		{"click on visual row 1 → line 0 wrap 1", 0, 4, 1, 2, 10, 0, 10},
		{"click on visual row 2 → line 0 wrap 2", 0, 4, 2, 2, 10, 0, 20},
		{"click on visual row 3 → line 1 wrap 0", 0, 4, 3, 2, 10, 1, 0},
		{"click on visual row 5 → line 1 wrap 2", 0, 4, 5, 2, 10, 1, 20},
		{"click on visual row 6 → line 2", 0, 4, 6, 2, 10, 2, 0},
		{"scrolled: yOffset=2, localY=1 → visual row 3 → line 1", 2, 4, 1, 2, 10, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := mousePosToEditorCursor(content, tt.visualYOffset, tt.localX, tt.localY, tt.numWidth, tt.wrapWidth)
			if line != tt.wantLine || col != tt.wantCol {
				t.Errorf("got (%d,%d), want (%d,%d)", line, col, tt.wantLine, tt.wantCol)
			}
		})
	}
}

func TestWrapContent_NoWrappingWhenFits(t *testing.T) {
	rows := wrapContent("hello\nworld", 20)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].line != 0 || rows[0].startCol != 0 || rows[0].length != 5 {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[1].line != 1 || rows[1].startCol != 0 || rows[1].length != 5 {
		t.Errorf("rows[1] = %+v", rows[1])
	}
}

func TestWrapContent_WordWrap(t *testing.T) {
	// "aaaa bbbb cccc dddd" wrapped at 10 cols. Greedy word wrap breaks at
	// the last space inside the 10-rune window, so:
	//   row 0: "aaaa bbbb " (10 runes, ends after the space at idx 9)
	//   row 1: "cccc dddd" (9 runes)
	rows := wrapContent("aaaa bbbb cccc dddd", 10)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].startCol != 0 || rows[0].length != 10 {
		t.Errorf("rows[0] = %+v, want startCol=0 length=10", rows[0])
	}
	if rows[1].startCol != 10 || rows[1].length != 9 {
		t.Errorf("rows[1] = %+v, want startCol=10 length=9", rows[1])
	}
}

func TestWrapContent_HardBreakLongWord(t *testing.T) {
	// 25-char word with no spaces → char breaks at wrapWidth.
	rows := wrapContent(strings.Repeat("a", 25), 10)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].startCol != 0 || rows[0].length != 10 {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[1].startCol != 10 || rows[1].length != 10 {
		t.Errorf("rows[1] = %+v", rows[1])
	}
	if rows[2].startCol != 20 || rows[2].length != 5 {
		t.Errorf("rows[2] = %+v", rows[2])
	}
}

func TestWrapContent_EmptyLinesKeptAsOneRow(t *testing.T) {
	rows := wrapContent("a\n\nb", 10)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[1].line != 1 || rows[1].length != 0 {
		t.Errorf("rows[1] = %+v, want empty on line 1", rows[1])
	}
}

func TestMousePosToTreeRow(t *testing.T) {
	if got := mousePosToTreeRow(0, 5); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
	if got := mousePosToTreeRow(10, 3); got != 13 {
		t.Errorf("got %d, want 13", got)
	}
}

func TestEditorNumWidth(t *testing.T) {
	tests := []struct {
		content string
		want    int
	}{
		{"single line", 2},
		{"one\ntwo\nthree", 2},
		{strings.Repeat("x\n", 98) + "x", 2},  // 99 lines → 2 digits
		{strings.Repeat("x\n", 99) + "x", 3},  // 100 lines → 3 digits
	}
	for _, tt := range tests {
		if got := editorNumWidth(tt.content); got != tt.want {
			t.Errorf("editorNumWidth(%d lines) = %d, want %d",
				strings.Count(tt.content, "\n")+1, got, tt.want)
		}
	}
}

func newMouseTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "")
	m.width = 100
	m.height = 30
	m.treeWidth = 20
	m.editorWidth = 79
	m.editorHeight = 29
	setEditorSize(&m.editor, m.editorWidth, m.editorHeight)
	return m
}

func TestHandleEditorMouse_PressPositionsCursor(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello world\nsecond line")
	m.activePanel = treePanel

	// Editor local (5, 0). Col = 5-2-2 = 1.
	msg := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, msg)
	nm := next.(model)
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", nm.activePanel)
	}
	if nm.editor.Line() != 0 || nm.editor.cursorCol() != 1 {
		t.Errorf("cursor = (%d,%d), want (0,1)", nm.editor.Line(), nm.editor.cursorCol())
	}
}

func TestHandleEditorMouse_DragSelects(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, press)
	m = next.(model)

	motion := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion}
	next, _ = handleEditorMouse(m, 9, 0, motion)
	m = next.(model)

	if !m.editor.selActive {
		t.Error("selActive should be true after motion")
	}
	got := m.editor.SelectedText()
	if got != "ello" {
		t.Errorf("SelectedText = %q, want %q", got, "ello")
	}

	release := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
	next, _ = handleEditorMouse(m, 9, 0, release)
	m = next.(model)
	if !m.editor.selActive {
		t.Error("selActive should persist after release with selection")
	}
}

func TestHandleEditorMouse_WheelScrolls(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("line1\nline2\nline3\nline4\nline5")
	m.editor.StartMouseDrag(3, 0)
	m.editor.EndMouseDrag()

	wheel := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 0, 0, wheel)
	m = next.(model)
	if m.editor.Line() != 0 {
		t.Errorf("line after wheel up = %d, want 0", m.editor.Line())
	}

	wheelDown := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	next, _ = handleEditorMouse(m, 0, 0, wheelDown)
	m = next.(model)
	if m.editor.Line() != 3 {
		t.Errorf("line after wheel down = %d, want 3", m.editor.Line())
	}
}

func TestHandleEditorMouse_PreviewModeClickFocusesEdit(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello")
	m.editorMode = modePreview
	m.activePanel = editorPanel

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleEditorMouse(m, 5, 0, press)
	m = next.(model)
	if m.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", m.editorMode)
	}
}

func newMouseTreeModel(t *testing.T) model {
	t.Helper()
	m := newMouseTestModel(t)
	vault := m.vault
	os.WriteFile(filepath.Join(vault, "alpha.md"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(vault, "beta.md"), []byte("beta"), 0o644)
	os.Mkdir(filepath.Join(vault, "sub"), 0o755)
	os.WriteFile(filepath.Join(vault, "sub", "c.md"), []byte("c"), 0o644)
	m.refreshTree()
	m.tree.height = 10
	m.tree.width = 20
	return m
}

func TestHandleTreeMouse_ClickFileSelectsAndPreviews(t *testing.T) {
	m := newMouseTreeModel(t)
	// Row 0 = "sub" directory, row 1 = "alpha.md"
	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleTreeMouse(m, 1, press)
	m = next.(model)
	if m.tree.cursor != 1 {
		t.Errorf("tree.cursor = %d, want 1", m.tree.cursor)
	}
	if m.currentFile == "" {
		t.Error("currentFile should be set after clicking a file")
	}
	if m.editorMode != modePreview {
		t.Errorf("editorMode = %v, want modePreview", m.editorMode)
	}
	if m.activePanel != treePanel {
		t.Errorf("activePanel = %v, want treePanel", m.activePanel)
	}
}

func TestHandleTreeMouse_ClickFolderToggles(t *testing.T) {
	m := newMouseTreeModel(t)
	node := m.tree.items[0].Node
	if !node.IsDir {
		t.Fatalf("expected row 0 to be a directory; got %+v", node)
	}
	initialExpanded := node.Expanded

	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleTreeMouse(m, 0, press)
	m = next.(model)
	if m.tree.items[0].Node.Expanded == initialExpanded {
		t.Error("folder expanded state should have toggled")
	}
}

func TestHandleTreeMouse_WheelScrolls(t *testing.T) {
	m := newMouseTreeModel(t)
	m.tree.offset = 0
	wheel := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	next, _ := handleTreeMouse(m, 0, wheel)
	m = next.(model)
	if m.tree.offset < 0 {
		t.Errorf("offset went negative: %d", m.tree.offset)
	}

	m.tree.offset = 5
	wheelUp := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}
	next, _ = handleTreeMouse(m, 0, wheelUp)
	m = next.(model)
	if m.tree.offset > 2 {
		t.Errorf("offset after wheel up = %d, want <= 2", m.tree.offset)
	}
}

func TestHandleTreeMouse_OutOfBoundsRowIgnored(t *testing.T) {
	m := newMouseTreeModel(t)
	press := tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	before := m.tree.cursor
	next, _ := handleTreeMouse(m, 99, press)
	m = next.(model)
	if m.tree.cursor != before {
		t.Errorf("cursor moved unexpectedly: before=%d after=%d", before, m.tree.cursor)
	}
}

func TestHandleMouseMsg_StatusBarIgnored(t *testing.T) {
	m := newMouseTestModel(t)
	before := m.activePanel
	msg := tea.MouseMsg{X: 10, Y: 29, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, cmd := handleMouseMsg(m, msg)
	m = next.(model)
	if cmd != nil {
		t.Error("status-bar click should return nil cmd")
	}
	if m.activePanel != before {
		t.Error("status-bar click should not change active panel")
	}
}

func TestHandleMouseMsg_PreviewWheelForwardsToViewport(t *testing.T) {
	m := newMouseTestModel(t)
	m.editorMode = modePreview
	m.activePanel = editorPanel
	msg := tea.MouseMsg{X: 50, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	next, _ := handleMouseMsg(m, msg)
	m = next.(model)
	if m.editorMode != modePreview {
		t.Error("preview wheel should not change editorMode")
	}
}

func TestHandleMouseMsg_RoutesToEditor(t *testing.T) {
	m := newMouseTestModel(t)
	m.editor.SetValue("hello")
	msg := tea.MouseMsg{X: 25, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := handleMouseMsg(m, msg)
	m = next.(model)
	if m.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", m.activePanel)
	}
}

func TestHandleTreeMouse_WheelDownScrollsPastCursor(t *testing.T) {
	m := newMouseTestModel(t)
	for i := 0; i < 50; i++ {
		os.WriteFile(filepath.Join(m.vault, fmt.Sprintf("f%02d.md", i)), []byte("x"), 0o644)
	}
	m.refreshTree()
	m.tree.height = 10
	m.tree.width = 20
	m.tree.cursor = 0
	m.tree.offset = 0

	wheel := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	for i := 0; i < 5; i++ {
		next, _ := handleTreeMouse(m, 0, wheel)
		m = next.(model)
	}
	if m.tree.offset < 9 {
		t.Errorf("after 5 wheel-down events: offset = %d, want >= 9 (decoupled from cursor)", m.tree.offset)
	}
	if m.tree.cursor != 0 {
		t.Errorf("cursor moved during wheel scroll: cursor = %d, want 0", m.tree.cursor)
	}
}
