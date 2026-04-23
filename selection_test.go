package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true}, {'Z', true}, {'0', true}, {'_', true},
		{' ', false}, {'.', false}, {'-', false}, {'\n', false},
	}
	for _, tt := range tests {
		if got := isWordChar(tt.r); got != tt.want {
			t.Errorf("isWordChar(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestWordLeftPos(t *testing.T) {
	tests := []struct {
		content  string
		line     int
		col      int
		wantLine int
		wantCol  int
	}{
		{"hello world", 0, 11, 0, 6},
		{"hello world", 0, 6, 0, 0},
		{"hello world", 0, 8, 0, 6},
		{"hello world", 0, 0, 0, 0},
		{"first\nsecond", 1, 0, 0, 0},
		{"hello  world", 0, 7, 0, 0},
	}
	for _, tt := range tests {
		gotLine, gotCol := wordLeftPos(tt.content, tt.line, tt.col)
		if gotLine != tt.wantLine || gotCol != tt.wantCol {
			t.Errorf("wordLeftPos(%q, %d, %d) = (%d, %d), want (%d, %d)",
				tt.content, tt.line, tt.col, gotLine, gotCol, tt.wantLine, tt.wantCol)
		}
	}
}

func TestWordRightPos(t *testing.T) {
	tests := []struct {
		content  string
		line     int
		col      int
		wantLine int
		wantCol  int
	}{
		{"hello world", 0, 0, 0, 6},
		{"hello world", 0, 6, 0, 11},
		{"hello world", 0, 3, 0, 6},
		{"first\nsecond", 0, 0, 0, 5},
		{"first\nsecond", 0, 5, 1, 0},
	}
	for _, tt := range tests {
		gotLine, gotCol := wordRightPos(tt.content, tt.line, tt.col)
		if gotLine != tt.wantLine || gotCol != tt.wantCol {
			t.Errorf("wordRightPos(%q, %d, %d) = (%d, %d), want (%d, %d)",
				tt.content, tt.line, tt.col, gotLine, gotCol, tt.wantLine, tt.wantCol)
		}
	}
}

func TestSelectionRange(t *testing.T) {
	sL, sC, eL, eC := selectionRange(0, 0, 1, 5)
	if sL != 0 || sC != 0 || eL != 1 || eC != 5 {
		t.Errorf("selectionRange(0,0,1,5) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
	sL, sC, eL, eC = selectionRange(1, 5, 0, 0)
	if sL != 0 || sC != 0 || eL != 1 || eC != 5 {
		t.Errorf("selectionRange(1,5,0,0) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
	sL, sC, eL, eC = selectionRange(2, 10, 2, 3)
	if sL != 2 || sC != 3 || eL != 2 || eC != 10 {
		t.Errorf("selectionRange(2,10,2,3) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
}

func TestExtractText(t *testing.T) {
	content := "hello world\nfoo bar\nbaz"
	tests := []struct {
		sLine, sCol, eLine, eCol int
		want                     string
	}{
		{0, 0, 0, 5, "hello"},
		{0, 6, 0, 11, "world"},
		{0, 0, 1, 3, "hello world\nfoo"},
		{0, 0, 2, 3, "hello world\nfoo bar\nbaz"},
		{1, 4, 2, 3, "bar\nbaz"},
	}
	for _, tt := range tests {
		got := extractText(content, tt.sLine, tt.sCol, tt.eLine, tt.eCol)
		if got != tt.want {
			t.Errorf("extractText(%d,%d,%d,%d) = %q, want %q",
				tt.sLine, tt.sCol, tt.eLine, tt.eCol, got, tt.want)
		}
	}
}

func TestDeleteText(t *testing.T) {
	content := "hello world\nfoo bar\nbaz"
	tests := []struct {
		sLine, sCol, eLine, eCol int
		want                     string
	}{
		{0, 5, 0, 11, "hello\nfoo bar\nbaz"},
		{0, 0, 0, 5, " world\nfoo bar\nbaz"},
		{0, 5, 1, 4, "hellobar\nbaz"},
		{0, 0, 2, 3, ""},
	}
	for _, tt := range tests {
		got := deleteText(content, tt.sLine, tt.sCol, tt.eLine, tt.eCol)
		if got != tt.want {
			t.Errorf("deleteText(%d,%d,%d,%d) = %q, want %q",
				tt.sLine, tt.sCol, tt.eLine, tt.eCol, got, tt.want)
		}
	}
}

func TestPosInRange(t *testing.T) {
	tests := []struct {
		line, col int
		want      bool
	}{
		{0, 0, false}, {0, 1, false}, {0, 2, true}, {0, 5, true},
		{1, 0, true}, {1, 2, true}, {1, 3, false}, {1, 4, false},
		{2, 0, false},
	}
	for _, tt := range tests {
		got := posInRange(tt.line, tt.col, 0, 2, 1, 3)
		if got != tt.want {
			t.Errorf("posInRange(%d, %d, 0,2,1,3) = %v, want %v", tt.line, tt.col, got, tt.want)
		}
	}
}

func TestMouseDragSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world\nsecond line")
	setEditorSize(&e, 80, 10)

	e.StartMouseDrag(0, 3)
	if e.selActive {
		t.Error("selActive should not be set on press without motion")
	}
	if e.Line() != 0 {
		t.Errorf("line after press = %d, want 0", e.Line())
	}

	e.UpdateMouseDrag(0, 8)
	if !e.selActive {
		t.Error("selActive should be true after motion to different position")
	}
	if e.selAnchorLine != 0 || e.selAnchorCol != 3 {
		t.Errorf("anchor = (%d,%d), want (0,3)", e.selAnchorLine, e.selAnchorCol)
	}
	got := e.SelectedText()
	if got != "lo wo" {
		t.Errorf("SelectedText = %q, want %q", got, "lo wo")
	}

	e.EndMouseDrag()
	if !e.selActive {
		t.Error("selActive should persist after release with non-empty selection")
	}
	if e.mouseDragging {
		t.Error("mouseDragging should be false after release")
	}
}

func TestMouseClickWithoutDragClearsSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	e.StartMouseDrag(0, 5)
	e.EndMouseDrag()
	if e.selActive {
		t.Error("selActive should be false after press+release without motion")
	}
	if e.mouseDragging {
		t.Error("mouseDragging should be false after release")
	}
}

func TestMouseDragBackToAnchorClearsSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	e.StartMouseDrag(0, 3)
	e.UpdateMouseDrag(0, 8)
	e.UpdateMouseDrag(0, 3)
	e.EndMouseDrag()
	if e.selActive {
		t.Error("selActive should be false after dragging back to anchor")
	}
}

func TestEditorScroll(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("line1\nline2\nline3\nline4\nline5")
	setEditorSize(&e, 80, 3)

	e.StartMouseDrag(3, 0)
	e.EndMouseDrag()
	e.ScrollUp(2)
	if e.Line() != 1 {
		t.Errorf("line after ScrollUp(2) = %d, want 1", e.Line())
	}

	e.ScrollDown(2)
	if e.Line() != 3 {
		t.Errorf("line after ScrollDown(2) = %d, want 3", e.Line())
	}
}

var _ = tea.KeyMsg{} // ensure tea import is used regardless of other tests

func TestHandleKey_DeleteSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)
	e.SetCursor(0) // SetValue leaves cursor at end; reset to start of line

	for i := 0; i < 5; i++ {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	if !e.selActive {
		t.Fatal("shift+right should activate selection")
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if e.Value() != " world" {
		t.Errorf("after delete-selection: Value = %q, want %q", e.Value(), " world")
	}
	if e.selActive {
		t.Error("selection should be cleared after delete")
	}
}

func TestHandleKey_DeleteMouseSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)

	e.StartMouseDrag(0, 0)
	e.UpdateMouseDrag(0, 5)
	e.EndMouseDrag()

	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if e.Value() != " world" {
		t.Errorf("after delete-mouse-selection: Value = %q, want %q", e.Value(), " world")
	}
}

func TestHandleKey_HomeEndMoveCursor(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)
	for i := 0; i < 5; i++ {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyHome})
	if e.cursorCol() != 0 {
		t.Errorf("after Home: col = %d, want 0", e.cursorCol())
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyEnd})
	if e.cursorCol() != len("hello world") {
		t.Errorf("after End: col = %d, want %d", e.cursorCol(), len("hello world"))
	}
}

func TestHandleKey_HomeClearsSelection(t *testing.T) {
	e := newSelectableEditor()
	e.SetValue("hello world")
	setEditorSize(&e, 80, 10)
	e.HandleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyShiftRight})
	if !e.selActive {
		t.Fatal("shift+right should activate selection")
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyHome})
	if e.selActive {
		t.Error("plain Home should clear selection")
	}
	if e.cursorCol() != 0 {
		t.Errorf("col after Home = %d, want 0", e.cursorCol())
	}
}

func TestSelectableEditor_SnapshotRoundTrip(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.SetValue("hello\nworld")
	e.moveTo(1, 3)
	snap := e.snapshotNow()
	if snap.content != "hello\nworld" {
		t.Fatalf("snapshot content = %q", snap.content)
	}
	if snap.line != 1 || snap.col != 3 {
		t.Fatalf("snapshot cursor = (%d,%d), want (1,3)", snap.line, snap.col)
	}

	e.SetValue("changed")
	e.moveTo(0, 0)
	e.restoreSnapshot(snap)
	if e.Value() != "hello\nworld" {
		t.Fatalf("after restore, Value = %q", e.Value())
	}
	if e.Line() != 1 || e.cursorCol() != 3 {
		t.Fatalf("after restore, cursor = (%d,%d)", e.Line(), e.cursorCol())
	}
}

func TestSelectableEditor_UndoRedoEmptyStacksNoOp(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.SetValue("abc")
	before := e.Value()
	if e.Undo() {
		t.Fatal("Undo on empty history should return false")
	}
	if e.Redo() {
		t.Fatal("Redo on empty history should return false")
	}
	if e.Value() != before {
		t.Fatal("no-op undo/redo should not mutate buffer")
	}
}

func TestSelectableEditor_ClearHistoryWipesStacks(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.history.recordBefore(editKindOp, snap("pre"))
	e.history.pushRedo(snap("redo"))
	e.ClearHistory()
	if len(e.history.undoStack) != 0 || len(e.history.redoStack) != 0 {
		t.Fatal("ClearHistory must empty both stacks")
	}
}

func typeRunes(e *SelectableEditor, s string) {
	for _, r := range s {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestSelectableEditor_TypingRunCoalesces(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	typeRunes(&e, "hello world")
	if !e.Undo() {
		t.Fatal("Undo should succeed")
	}
	if e.Value() != "" {
		t.Fatalf("after undo, Value = %q, want empty", e.Value())
	}
	if e.Redo() != true || e.Value() != "hello world" {
		t.Fatalf("after redo, Value = %q", e.Value())
	}
}

func TestSelectableEditor_CursorMoveBreaksGroup(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	typeRunes(&e, "a")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	typeRunes(&e, "b")
	if !e.Undo() {
		t.Fatal("first undo should succeed")
	}
	if e.Value() != "a" {
		t.Fatalf("after first undo, Value = %q, want \"a\"", e.Value())
	}
	if !e.Undo() {
		t.Fatal("second undo should succeed")
	}
	if e.Value() != "" {
		t.Fatalf("after second undo, Value = %q, want empty", e.Value())
	}
}

func TestSelectableEditor_TypingThenDeletingSplitsGroup(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	typeRunes(&e, "ab")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if e.Value() != "" {
		t.Fatalf("after backspaces, Value = %q", e.Value())
	}
	if !e.Undo() {
		t.Fatal("first undo should succeed (undo delete group)")
	}
	if e.Value() != "ab" {
		t.Fatalf("after undo delete group, Value = %q", e.Value())
	}
	if !e.Undo() {
		t.Fatal("second undo should succeed (undo typing group)")
	}
	if e.Value() != "" {
		t.Fatalf("after undo typing group, Value = %q", e.Value())
	}
}

func TestSelectableEditor_NoOpBackspaceAtStartNoHistoryEntry(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if len(e.history.undoStack) != 0 {
		t.Fatalf("undoStack after no-op backspace: %d entries, want 0", len(e.history.undoStack))
	}
}

func TestSelectableEditor_DeleteSelectionIsUndoable(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.SetValue("hello world")
	e.moveTo(0, 0)
	e.selAnchorLine, e.selAnchorCol, e.selActive = 0, 0, true
	e.moveTo(0, 5)
	e.DeleteSelection()
	if e.Value() != " world" {
		t.Fatalf("after DeleteSelection, Value = %q", e.Value())
	}
	if !e.Undo() {
		t.Fatal("Undo should succeed after DeleteSelection")
	}
	if e.Value() != "hello world" {
		t.Fatalf("after undo, Value = %q", e.Value())
	}
}

func TestSelectableEditor_ReplaceSelectionIsUndoable(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.SetValue("foo bar")
	e.moveTo(0, 0)
	e.selAnchorLine, e.selAnchorCol, e.selActive = 0, 0, true
	e.moveTo(0, 3)
	e.ReplaceSelection("baz")
	if e.Value() != "baz bar" {
		t.Fatalf("after ReplaceSelection, Value = %q", e.Value())
	}
	if !e.Undo() {
		t.Fatal("Undo should succeed after ReplaceSelection")
	}
	if e.Value() != "foo bar" {
		t.Fatalf("after undo, Value = %q", e.Value())
	}
}

func TestSelectableEditor_CutIsUndoable(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.SetValue("abc")
	e.moveTo(0, 0)
	e.selAnchorLine, e.selAnchorCol, e.selActive = 0, 0, true
	e.moveTo(0, 3)
	e.Cut()
	if e.Value() != "" {
		t.Fatalf("after Cut, Value = %q", e.Value())
	}
	if !e.Undo() {
		t.Fatal("Undo should succeed after Cut")
	}
	if e.Value() != "abc" {
		t.Fatalf("after undo, Value = %q", e.Value())
	}
}

func TestSelectableEditor_OpsEachPushOwnGroup(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	e.SetValue("hello world")

	e.moveTo(0, 0)
	e.selAnchorLine, e.selAnchorCol, e.selActive = 0, 0, true
	e.moveTo(0, 5)
	e.DeleteSelection()

	e.moveTo(0, 0)
	e.selAnchorLine, e.selAnchorCol, e.selActive = 0, 0, true
	e.moveTo(0, 6)
	e.ReplaceSelection("X")

	if e.Value() != "X" {
		t.Fatalf("Value = %q, want X", e.Value())
	}
	e.Undo()
	if e.Value() != " world" {
		t.Fatalf("after 1st undo, Value = %q", e.Value())
	}
	e.Undo()
	if e.Value() != "hello world" {
		t.Fatalf("after 2nd undo, Value = %q", e.Value())
	}
}

func TestSelectableEditor_CtrlZUndos(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	typeRunes(&e, "hello")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlZ})
	if e.Value() != "" {
		t.Fatalf("after Ctrl+Z, Value = %q", e.Value())
	}
}

func TestSelectableEditor_CtrlShiftZRedos(t *testing.T) {
	t.Skip("bubbletea KeyMsg does not surface ctrl+shift+z distinctly; covered by Ctrl+Y test")
}

func TestSelectableEditor_CtrlYRedos(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	typeRunes(&e, "hello")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlZ})
	e.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	if e.Value() != "hello" {
		t.Fatalf("after Ctrl+Y redo, Value = %q", e.Value())
	}
}

func TestSelectableEditor_NewEditAfterUndoClearsRedo(t *testing.T) {
	e := newSelectableEditor()
	setEditorSize(&e, 80, 10)
	typeRunes(&e, "a")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlZ})
	typeRunes(&e, "b")
	e.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	if e.Value() != "b" {
		t.Fatalf("after new edit + Ctrl+Y, Value = %q, want \"b\"", e.Value())
	}
}
