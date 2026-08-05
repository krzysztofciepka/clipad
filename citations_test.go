package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestNextCiteIndex(t *testing.T) {
	tests := []struct {
		name             string
		cursor, delta, n int
		want             int
	}{
		{"no citations", 0, 1, 0, 0},
		{"first next from none", 0, 1, 3, 1},
		{"first prev from none", 0, -1, 3, 3},
		{"next", 1, 1, 3, 2},
		{"prev", 2, -1, 3, 1},
		{"next wraps", 3, 1, 3, 1},
		{"prev wraps", 1, -1, 3, 3},
		{"single citation next", 1, 1, 1, 1},
		{"single citation prev", 1, -1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextCiteIndex(tt.cursor, tt.delta, tt.n); got != tt.want {
				t.Errorf("nextCiteIndex(%d,%d,%d) = %d, want %d",
					tt.cursor, tt.delta, tt.n, got, tt.want)
			}
		})
	}
}

func TestChatCitations(t *testing.T) {
	turns := []chatTurn{
		{Role: "assistant", Citations: []citation{{Path: "old.md"}}},
		{Role: "user", Content: "again"},
		{Role: "assistant", Citations: []citation{{Path: "new1.md"}, {Path: "new2.md"}}},
	}
	got := chatCitations(turns)
	if len(got) != 2 || got[0].Path != "new1.md" {
		t.Errorf("chatCitations returned %v, want the newest turn's two citations", got)
	}
	if chatCitations(nil) != nil {
		t.Error("chatCitations(nil) should be nil")
	}
	if chatCitations([]chatTurn{{Role: "assistant"}}) != nil {
		t.Error("a turn with no citations should yield nil")
	}
}

// jumpCitationModel builds a model with a real vault file so jumpCitation can
// open it.
func jumpCitationModel(t *testing.T, cites []citation) model {
	t.Helper()
	dir := t.TempDir()
	for _, c := range cites {
		p := filepath.Join(dir, c.Path)
		if err := os.WriteFile(p, []byte("line one\nline two\nline three\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return model{
		vault:     dir,
		editor:    newSelectableEditor(),
		chatOpen:  true,
		chatTurns: []chatTurn{{Role: "assistant", Citations: cites}},
	}
}

func TestJumpCitationOpensFiles(t *testing.T) {
	cites := []citation{
		{Path: "a.md", StartLine: 2, EndLine: 2},
		{Path: "b.md", StartLine: 3, EndLine: 3},
	}

	t.Run("next from none opens the first", func(t *testing.T) {
		m := jumpCitationModel(t, cites)
		m.jumpCitation(1)
		if m.chatCiteCursor != 1 {
			t.Errorf("cursor = %d, want 1", m.chatCiteCursor)
		}
		if filepath.Base(m.currentFile) != "a.md" {
			t.Errorf("currentFile = %q, want a.md", m.currentFile)
		}
		if line, _ := editorCursorPos(m.editor); line != 1 {
			t.Errorf("cursor line = %d, want 1 (0-based for StartLine 2)", line)
		}
	})

	t.Run("prev from none opens the last", func(t *testing.T) {
		m := jumpCitationModel(t, cites)
		m.jumpCitation(-1)
		if m.chatCiteCursor != 2 {
			t.Errorf("cursor = %d, want 2", m.chatCiteCursor)
		}
		if filepath.Base(m.currentFile) != "b.md" {
			t.Errorf("currentFile = %q, want b.md", m.currentFile)
		}
	})

	t.Run("no citations is a no-op", func(t *testing.T) {
		m := model{editor: newSelectableEditor(), chatOpen: true}
		m.jumpCitation(1)
		if m.chatCiteCursor != 0 || m.currentFile != "" {
			t.Error("expected no state change")
		}
	})

	t.Run("dirty buffer detours through the unsaved guard", func(t *testing.T) {
		m := jumpCitationModel(t, cites)
		m.editor.SetValue("unsaved edits")
		m.cleanContent = ""
		m.jumpCitation(1)
		if m.inputMode != inputUnsavedGuard {
			t.Fatalf("inputMode = %v, want inputUnsavedGuard", m.inputMode)
		}
		if m.pendingAction != pendingSwitchFile {
			t.Errorf("pendingAction = %v, want pendingSwitchFile", m.pendingAction)
		}
		if filepath.Base(m.pendingSwitchPath) != "a.md" {
			t.Errorf("pendingSwitchPath = %q, want a.md", m.pendingSwitchPath)
		}
	})
}

func TestChatFocusInput(t *testing.T) {
	m := model{chatOpen: true, chatMode: chatModeView, buttonFocused: true, chatInput: textinput.New()}
	m.chatFocusInput()
	if m.chatMode != chatModeInput {
		t.Errorf("chatMode = %v, want chatModeInput", m.chatMode)
	}
	if m.buttonFocused {
		t.Error("bar focus should move to the chat input")
	}

	closed := model{chatOpen: false, chatMode: chatModeView}
	closed.chatFocusInput()
	if closed.chatMode != chatModeView {
		t.Error("chatFocusInput must be a no-op when the panel is closed")
	}
}
