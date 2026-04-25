package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func pressEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func pressEsc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }

func newTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	return newModel(vault, nil, "")
}

func TestShortcutFlow_Create_GoesThroughDescriptionStep(t *testing.T) {
	m := newTestModel(t)
	m.shortcutEditing = -1
	m.inputMode = inputShortcutName
	m.shortcutNameInput.SetValue("mytest")

	next, _ := m.handleShortcutName(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputShortcutDescription {
		t.Fatalf("after name: inputMode = %v, want inputShortcutDescription", nm.inputMode)
	}
	if nm.shortcutTempName != "mytest" {
		t.Errorf("shortcutTempName = %q, want %q", nm.shortcutTempName, "mytest")
	}

	nm.shortcutDescriptionInput.SetValue("")
	next2, _ := nm.handleShortcutDescription(pressEnter())
	nm2 := next2.(model)
	if nm2.inputMode != inputShortcutDescription {
		t.Errorf("empty description should block advance, got %v", nm2.inputMode)
	}

	nm2.shortcutDescriptionInput.SetValue("short desc")
	next3, _ := nm2.handleShortcutDescription(pressEnter())
	nm3 := next3.(model)
	if nm3.inputMode != inputShortcutPrompt {
		t.Fatalf("after description: inputMode = %v, want inputShortcutPrompt", nm3.inputMode)
	}
	if nm3.shortcutTempDescription != "short desc" {
		t.Errorf("shortcutTempDescription = %q, want %q", nm3.shortcutTempDescription, "short desc")
	}

	nm3.shortcutPromptInput.SetValue("do the thing")
	next4, _ := nm3.handleShortcutPrompt(pressEnter())
	nm4 := next4.(model)
	if nm4.inputMode != inputNone {
		t.Errorf("after prompt: inputMode = %v, want inputNone", nm4.inputMode)
	}
	found := false
	for _, s := range nm4.shortcuts {
		if s.Name == "mytest" {
			found = true
			if s.Description != "short desc" {
				t.Errorf("saved description = %q, want %q", s.Description, "short desc")
			}
			if s.Prompt != "do the thing" {
				t.Errorf("saved prompt = %q, want %q", s.Prompt, "do the thing")
			}
		}
	}
	if !found {
		t.Error("new shortcut 'mytest' not saved")
	}
}

func TestShortcutFlow_Edit_PrefillsDescription(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{
		{Name: "n1", Description: "d1", Prompt: "p1"},
	}
	m.shortcutCursor = 0
	m.shortcutEditing = 0
	m.inputMode = inputShortcutName
	m.shortcutNameInput.SetValue("n1")

	next, _ := m.handleShortcutName(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputShortcutDescription {
		t.Fatalf("inputMode = %v, want inputShortcutDescription", nm.inputMode)
	}
	if got := nm.shortcutDescriptionInput.Value(); got != "d1" {
		t.Errorf("description input not prefilled: got %q, want %q", got, "d1")
	}
}

func TestShortcutFlow_Description_EscCancels(t *testing.T) {
	m := newTestModel(t)
	m.shortcutEditing = 3
	m.inputMode = inputShortcutDescription
	m.shortcutDescriptionInput.SetValue("typed something")

	next, _ := m.handleShortcutDescription(pressEsc())
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("after esc: inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.shortcutEditing != -1 {
		t.Errorf("after esc: shortcutEditing = %d, want -1", nm.shortcutEditing)
	}
}

func TestShortcutSelector_CtrlDown_SwapsAndPersists(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{
		{Name: "first", Description: "d1", Prompt: "p1"},
		{Name: "second", Description: "d2", Prompt: "p2"},
	}
	if err := saveShortcuts(m.shortcuts); err != nil {
		t.Fatalf("seed save failed: %v", err)
	}
	m.shortcutCursor = 0
	m.inputMode = inputShortcutSelect

	ctrlDown := tea.KeyMsg{Type: tea.KeyCtrlDown}
	next, _ := m.handleShortcutSelect(ctrlDown)
	nm := next.(model)
	if nm.shortcuts[0].Name != "second" || nm.shortcuts[1].Name != "first" {
		t.Errorf("order = [%s, %s], want [second, first]",
			nm.shortcuts[0].Name, nm.shortcuts[1].Name)
	}
	if nm.shortcutCursor != 1 {
		t.Errorf("cursor = %d, want 1", nm.shortcutCursor)
	}
	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts: %v", err)
	}
	if loaded[0].Name != "second" || loaded[1].Name != "first" {
		t.Errorf("on-disk order = [%s, %s], want [second, first]",
			loaded[0].Name, loaded[1].Name)
	}
}

func TestShortcutSelector_CtrlUp_SwapsAndPersists(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{
		{Name: "first", Description: "d1", Prompt: "p1"},
		{Name: "second", Description: "d2", Prompt: "p2"},
	}
	if err := saveShortcuts(m.shortcuts); err != nil {
		t.Fatalf("seed save failed: %v", err)
	}
	m.shortcutCursor = 1
	m.inputMode = inputShortcutSelect

	ctrlUp := tea.KeyMsg{Type: tea.KeyCtrlUp}
	next, _ := m.handleShortcutSelect(ctrlUp)
	nm := next.(model)
	if nm.shortcuts[0].Name != "second" || nm.shortcuts[1].Name != "first" {
		t.Errorf("order = [%s, %s], want [second, first]",
			nm.shortcuts[0].Name, nm.shortcuts[1].Name)
	}
	if nm.shortcutCursor != 0 {
		t.Errorf("cursor = %d, want 0", nm.shortcutCursor)
	}
}

func TestShortcutSelector_CtrlUp_AtTop_NoOp(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{
		{Name: "first"},
		{Name: "second"},
	}
	m.shortcutCursor = 0
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(tea.KeyMsg{Type: tea.KeyCtrlUp})
	nm := next.(model)
	if nm.shortcuts[0].Name != "first" {
		t.Errorf("ctrl+up at top should not swap; got %s", nm.shortcuts[0].Name)
	}
	if nm.shortcutCursor != 0 {
		t.Errorf("cursor = %d, want 0", nm.shortcutCursor)
	}
}

func TestShortcutSelector_CtrlDown_AtBottom_NoOp(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{
		{Name: "first"},
		{Name: "second"},
	}
	m.shortcutCursor = 1
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(tea.KeyMsg{Type: tea.KeyCtrlDown})
	nm := next.(model)
	if nm.shortcuts[1].Name != "second" {
		t.Errorf("ctrl+down at bottom should not swap; got %s", nm.shortcuts[1].Name)
	}
	if nm.shortcutCursor != 1 {
		t.Errorf("cursor = %d, want 1", nm.shortcutCursor)
	}
}
