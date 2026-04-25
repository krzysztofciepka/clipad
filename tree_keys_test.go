package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleTreeKeys_EnterOnAddNote_TriggersNewNote(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "")
	m.width = 100
	m.height = 30
	m.recalcLayout()
	m.tree.cursor = -1

	enter := tea.KeyMsg{Type: tea.KeyEnter}
	next, _ := m.handleTreeKeys(enter)
	nm := next.(model)
	if nm.newNoteDir == "" {
		t.Errorf("newNoteDir empty after Enter on Add note; want vault path")
	}
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v after Enter on Add note, want editorPanel", nm.activePanel)
	}
}
