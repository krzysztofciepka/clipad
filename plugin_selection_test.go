package main

import (
	"testing"
)

func TestAIInputContent_NoSelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	content, onSelection := m.aiInputContent()
	if content != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
	if onSelection {
		t.Error("onSelection = true, want false")
	}
}

func TestAIInputContent_WithSelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	content, onSelection := m.aiInputContent()
	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	if !onSelection {
		t.Error("onSelection = false, want true")
	}
}
