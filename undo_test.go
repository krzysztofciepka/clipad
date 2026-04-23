package main

import "testing"

func snap(content string) snapshot {
	return snapshot{content: content}
}

func TestEditHistory_RecordBeforeNewKindPushes(t *testing.T) {
	h := &editHistory{}
	pushed := h.recordBefore(editKindTyping, snap("A"))
	if !pushed {
		t.Fatal("first recordBefore should push")
	}
	if len(h.undoStack) != 1 || h.undoStack[0].content != "A" {
		t.Fatalf("undoStack = %+v, want 1 entry with content A", h.undoStack)
	}
	if h.activeKind != editKindTyping {
		t.Fatalf("activeKind = %v, want typing", h.activeKind)
	}
}

func TestEditHistory_RecordBeforeSameKindCoalesces(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindTyping, snap("A"))
	pushed := h.recordBefore(editKindTyping, snap("AB"))
	if pushed {
		t.Fatal("same-kind typing should coalesce (no push)")
	}
	if len(h.undoStack) != 1 {
		t.Fatalf("undoStack len = %d, want 1", len(h.undoStack))
	}
}

func TestEditHistory_RecordBeforeKindTransitionPushes(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindTyping, snap("A"))
	pushed := h.recordBefore(editKindDeleting, snap("ABC"))
	if !pushed {
		t.Fatal("typing -> deleting should push a new group")
	}
	if len(h.undoStack) != 2 {
		t.Fatalf("undoStack len = %d, want 2", len(h.undoStack))
	}
}

func TestEditHistory_OpAlwaysPushes(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindOp, snap("A"))
	pushed := h.recordBefore(editKindOp, snap("B"))
	if !pushed {
		t.Fatal("consecutive ops must each push their own group")
	}
	if len(h.undoStack) != 2 {
		t.Fatalf("undoStack len = %d, want 2", len(h.undoStack))
	}
}

func TestEditHistory_MovementBreaksGroup(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindTyping, snap("A"))
	h.breakGroup()
	pushed := h.recordBefore(editKindTyping, snap("AB"))
	if !pushed {
		t.Fatal("typing after breakGroup should push a new group")
	}
	if len(h.undoStack) != 2 {
		t.Fatalf("undoStack len = %d, want 2", len(h.undoStack))
	}
}

func TestEditHistory_PopUndoReturnsLIFO(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindOp, snap("A"))
	h.recordBefore(editKindOp, snap("B"))
	s, ok := h.popUndo()
	if !ok || s.content != "B" {
		t.Fatalf("popUndo = %+v, %v; want B, true", s, ok)
	}
	s, ok = h.popUndo()
	if !ok || s.content != "A" {
		t.Fatalf("popUndo = %+v, %v; want A, true", s, ok)
	}
	_, ok = h.popUndo()
	if ok {
		t.Fatal("popUndo on empty stack should return false")
	}
}

func TestEditHistory_PushOnUndoStackClearsRedo(t *testing.T) {
	h := &editHistory{}
	h.redoStack = []snapshot{snap("X"), snap("Y")}
	h.recordBefore(editKindOp, snap("A"))
	if len(h.redoStack) != 0 {
		t.Fatalf("redoStack = %+v, want empty after new edit", h.redoStack)
	}
}

func TestEditHistory_CapBounded(t *testing.T) {
	h := &editHistory{}
	for i := 0; i < maxHistory+5; i++ {
		h.recordBefore(editKindOp, snap("x"))
	}
	if len(h.undoStack) != maxHistory {
		t.Fatalf("undoStack len = %d, want %d", len(h.undoStack), maxHistory)
	}
}

func TestEditHistory_RevertLastPush(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindOp, snap("A"))
	h.revertLastPush()
	if len(h.undoStack) != 0 {
		t.Fatalf("undoStack len = %d, want 0 after revert", len(h.undoStack))
	}
	if h.activeKind != editKindNone {
		t.Fatalf("activeKind = %v, want none", h.activeKind)
	}
}

func TestEditHistory_ClearEmptiesBoth(t *testing.T) {
	h := &editHistory{}
	h.recordBefore(editKindOp, snap("A"))
	h.redoStack = []snapshot{snap("R")}
	h.clear()
	if len(h.undoStack) != 0 || len(h.redoStack) != 0 || h.activeKind != editKindNone {
		t.Fatalf("after clear: undo=%d redo=%d kind=%v", len(h.undoStack), len(h.redoStack), h.activeKind)
	}
}
