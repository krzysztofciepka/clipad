package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newToggleTestModel(t *testing.T, width int) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "", "")
	m.width = width
	m.height = 30
	m.recalcLayout()
	return m
}

func TestCtrlB_TogglesTreeHidden(t *testing.T) {
	m := newToggleTestModel(t, 100)
	if m.treeWidth == 0 {
		t.Fatalf("precondition: treeWidth should be > 0 at width=100; got 0")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	nm := next.(model)
	if !nm.treeHidden {
		t.Error("treeHidden should be true after Ctrl+B")
	}
	if nm.treeWidth != 0 {
		t.Errorf("treeWidth = %d after toggle hide, want 0", nm.treeWidth)
	}
	if nm.editorWidth != nm.width {
		t.Errorf("editorWidth = %d, want %d (full width)", nm.editorWidth, nm.width)
	}

	next2, _ := nm.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	nm2 := next2.(model)
	if nm2.treeHidden {
		t.Error("treeHidden should be false after second Ctrl+B")
	}
	if nm2.treeWidth == 0 {
		t.Errorf("treeWidth = %d after second toggle, want > 0", nm2.treeWidth)
	}
}

func TestCtrlB_HideWhileTreeFocused_FocusFollowsToEditor(t *testing.T) {
	m := newToggleTestModel(t, 100)
	m.activePanel = treePanel
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	nm := next.(model)
	if nm.activePanel != editorPanel {
		t.Errorf("activePanel = %v after hide, want editorPanel", nm.activePanel)
	}
}

func TestCtrlB_NarrowTerminal_StaysHiddenAfterUnHide(t *testing.T) {
	m := newToggleTestModel(t, 25) // below minTreeWidth+10 = 30
	if m.treeWidth != 0 {
		t.Fatalf("precondition: narrow terminal should auto-hide tree; treeWidth=%d", m.treeWidth)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = next.(model)
	next2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	nm := next2.(model)
	if nm.treeWidth != 0 {
		t.Errorf("after toggling on narrow terminal, treeWidth = %d, want 0", nm.treeWidth)
	}
}

func TestCtrlB_IgnoredDuringInputMode(t *testing.T) {
	m := newToggleTestModel(t, 100)
	m.inputMode = inputFilter
	beforeWidth := m.treeWidth
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	nm := next.(model)
	if nm.treeWidth != beforeWidth {
		t.Errorf("treeWidth changed during inputMode: was %d, now %d", beforeWidth, nm.treeWidth)
	}
	if nm.treeHidden {
		t.Error("treeHidden should not toggle during inputMode")
	}
}
