package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// selectHello puts "hello world" in the editor with "hello" selected and the
// editor panel focused, ready for a copy or cut key.
func selectHello(t *testing.T) model {
	t.Helper()
	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	m.editor.StartMouseDrag(0, 0)
	m.editor.UpdateMouseDrag(0, 5)
	m.editor.EndMouseDrag()
	if m.editor.SelectedText() != "hello" {
		t.Fatalf("setup: SelectedText = %q, want %q", m.editor.SelectedText(), "hello")
	}
	return m
}

func TestCtrlCCopiesAndFlashes(t *testing.T) {
	var wrote string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { wrote = s; return nil }

	m := selectHello(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(model)

	if wrote != "hello" {
		t.Errorf("clipboard = %q, want %q", wrote, "hello")
	}
	if !nm.copyFlash {
		t.Error("copyFlash should be set after ctrl+c")
	}
	if cmd == nil {
		t.Error("ctrl+c should return the flash fade tick")
	}
}

func TestCtrlXCutsAndFlashes(t *testing.T) {
	var wrote string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { wrote = s; return nil }

	m := selectHello(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	nm := next.(model)

	if wrote != "hello" {
		t.Errorf("clipboard = %q, want %q", wrote, "hello")
	}
	if nm.editor.Value() != " world" {
		t.Errorf("buffer after cut = %q, want %q", nm.editor.Value(), " world")
	}
	if !nm.copyFlash {
		t.Error("copyFlash should be set after ctrl+x")
	}
	if cmd == nil {
		t.Error("ctrl+x should return the flash fade tick")
	}
}

// TestViewShowsCopiedFlashThroughRealWiring drives the actual
// model.View() -> StatusBar wiring (sb.flashMsg = flashText(...)), unlike
// statusbar_test.go's TestViewShowsCopiedFlash which hand-builds a StatusBar
// and so passes regardless of whether model.go wires copyFlash correctly.
func TestViewShowsCopiedFlashThroughRealWiring(t *testing.T) {
	m := newMouseTestModel(t)
	m.copyFlash = true

	out := m.View()
	if !strings.Contains(out, "Copied") {
		t.Errorf("View() = %q, want it to contain %q", out, "Copied")
	}
}

// TestViewShowsCopiedFlashOverStickyErrMsg is the regression test for the
// bug where the status bar rendered errMsg instead of flashMsg whenever
// errMsg was non-empty. errMsg is sticky (only cleared on file open/save),
// so a lingering message like "select text first" from an earlier action
// would otherwise permanently suppress the green Copied flash until the
// user switched files. flashCopied() must clear errMsg so a fresh copy
// always shows. This drives the real path (stale errMsg -> ctrl+c ->
// model.Update -> flashCopied -> View), so it fails if the errMsg-clearing
// line in flashCopied() is reverted: statusbar.go's errMsg-wins-over-
// flashMsg priority is unchanged by design, so only clearing errMsg at the
// source makes the flash win here.
func TestViewShowsCopiedFlashOverStickyErrMsg(t *testing.T) {
	var wrote string
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(s string) error { wrote = s; return nil }

	m := selectHello(t)
	m.errMsg = "select text first" // stale message left over from an earlier action

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(model)

	if wrote != "hello" {
		t.Fatalf("setup: clipboard = %q, want %q", wrote, "hello")
	}
	out := nm.View()
	if !strings.Contains(out, "Copied") {
		t.Errorf("View() = %q, want it to contain %q even with a lingering errMsg set", out, "Copied")
	}
}

func TestCtrlCWithoutSelectionDoesNotFlash(t *testing.T) {
	var called bool
	old := writeClipboard
	defer func() { writeClipboard = old }()
	writeClipboard = func(string) error { called = true; return nil }

	m := newMouseTestModel(t)
	m.editor.SetValue("hello world")
	m.activePanel = editorPanel
	m.editorMode = modeEdit

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(model)

	if called {
		t.Error("ctrl+c without a selection should not write the clipboard")
	}
	if nm.copyFlash {
		t.Error("ctrl+c without a selection should not flash")
	}
	if cmd != nil {
		t.Error("ctrl+c without a selection should return no command")
	}
}
