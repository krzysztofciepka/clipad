package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpContent_NotEmpty(t *testing.T) {
	out := helpContent(80)
	if out == "" {
		t.Error("helpContent returned empty string")
	}
	if !strings.Contains(out, "Save") {
		t.Errorf("helpContent missing expected entry 'Save':\n%s", out)
	}
}

func newHelpTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	m := newModel(vault, nil, "", "")
	m.width = 100
	m.height = 10
	m.recalcLayout()
	return m
}

func openHelp(m model) model {
	// Replicate the ctrl+? handler without depending on a key constant whose
	// String() representation is "backspace" rather than "ctrl+?" in this
	// version of bubbletea.
	vp := viewport.New(m.editorWidth, m.editorHeight)
	vp.SetContent(helpContent(m.editorWidth))
	m.helpViewport = vp
	m.inputMode = inputHelp
	return m
}

func TestHelpModal_DownArrowScrollsViewport(t *testing.T) {
	m := openHelp(newHelpTestModel(t))
	startY := m.helpViewport.YOffset
	for i := 0; i < 5; i++ {
		next, _ := m.handleHelp(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(model)
	}
	if m.helpViewport.YOffset <= startY {
		t.Errorf("YOffset did not advance after Down keys: start=%d, end=%d",
			startY, m.helpViewport.YOffset)
	}
}

func TestHelpModal_EscClosesModal(t *testing.T) {
	m := openHelp(newHelpTestModel(t))
	next, _ := m.handleHelp(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v after Esc, want inputNone", nm.inputMode)
	}
}

func TestHelpModal_WheelScrollsViewport(t *testing.T) {
	m := openHelp(newHelpTestModel(t))
	startY := m.helpViewport.YOffset
	wheel := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, X: 0, Y: 0}
	next, _ := handleMouseMsg(m, wheel)
	m = next.(model)
	if m.helpViewport.YOffset <= startY {
		t.Errorf("wheel did not scroll help viewport: start=%d, end=%d",
			startY, m.helpViewport.YOffset)
	}
}

func TestHelpContent_IncludesReviewMode(t *testing.T) {
	out := helpContent(80)
	if !strings.Contains(out, "Review View") {
		t.Errorf("help content missing Review View section:\n%s", out)
	}
	if !strings.Contains(out, "switch pane") && !strings.Contains(out, "Switch pane") {
		t.Errorf("help content missing review Tab/switch-pane key:\n%s", out)
	}
}
