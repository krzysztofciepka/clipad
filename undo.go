package main

const maxHistory = 100

type editKind int

const (
	editKindNone editKind = iota
	editKindTyping
	editKindDeleting
	editKindOp
)

type snapshot struct {
	content       string
	line, col     int
	selActive     bool
	selAnchorLine int
	selAnchorCol  int
}

type editHistory struct {
	undoStack  []snapshot
	redoStack  []snapshot
	activeKind editKind
}

// recordBefore pushes pre onto the undo stack if the edit starts a new group.
// Returns true if a new group was started (i.e. pre was pushed).
// Consecutive same-kind typing/deleting coalesces and returns false.
// editKindOp always starts a new group.
func (h *editHistory) recordBefore(kind editKind, pre snapshot) bool {
	if h.activeKind == kind && kind != editKindOp {
		return false
	}
	h.pushUndo(pre)
	h.redoStack = nil
	h.activeKind = kind
	return true
}

func (h *editHistory) pushUndo(s snapshot) {
	h.undoStack = append(h.undoStack, s)
	if len(h.undoStack) > maxHistory {
		h.undoStack = h.undoStack[len(h.undoStack)-maxHistory:]
	}
}

func (h *editHistory) pushRedo(s snapshot) {
	h.redoStack = append(h.redoStack, s)
	if len(h.redoStack) > maxHistory {
		h.redoStack = h.redoStack[len(h.redoStack)-maxHistory:]
	}
}

func (h *editHistory) popUndo() (snapshot, bool) {
	if len(h.undoStack) == 0 {
		return snapshot{}, false
	}
	s := h.undoStack[len(h.undoStack)-1]
	h.undoStack = h.undoStack[:len(h.undoStack)-1]
	return s, true
}

func (h *editHistory) popRedo() (snapshot, bool) {
	if len(h.redoStack) == 0 {
		return snapshot{}, false
	}
	s := h.redoStack[len(h.redoStack)-1]
	h.redoStack = h.redoStack[:len(h.redoStack)-1]
	return s, true
}

// revertLastPush undoes the most recent recordBefore push (used when the
// corresponding edit turned out to be a no-op).
func (h *editHistory) revertLastPush() {
	if len(h.undoStack) > 0 {
		h.undoStack = h.undoStack[:len(h.undoStack)-1]
	}
	h.activeKind = editKindNone
}

// breakGroup forces the next recordBefore to start a new group regardless of
// kind. Called on cursor movement, undo, and redo.
func (h *editHistory) breakGroup() {
	h.activeKind = editKindNone
}

func (h *editHistory) clear() {
	h.undoStack = nil
	h.redoStack = nil
	h.activeKind = editKindNone
}
