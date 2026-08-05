package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestStartFindReplace(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(m *model)
		wantMode inputMode
	}{
		{"no note open is a no-op", func(m *model) {}, inputNone},
		{"open file enters replace search", func(m *model) { m.currentFile = "/v/a.md" }, inputReplaceSearch},
		{"new note enters replace search", func(m *model) { m.newNoteDir = "/v" }, inputReplaceSearch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{replaceSearchInput: textinput.New()}
			tt.setup(&m)
			m.replaceSearchInput.SetValue("stale")
			m.replaceSearchTerm = "stale"
			m.startFindReplace()
			if m.inputMode != tt.wantMode {
				t.Fatalf("inputMode = %v, want %v", m.inputMode, tt.wantMode)
			}
			if tt.wantMode == inputReplaceSearch {
				if m.replaceSearchInput.Value() != "" || m.replaceSearchTerm != "" {
					t.Errorf("stale search state not cleared: %q / %q",
						m.replaceSearchInput.Value(), m.replaceSearchTerm)
				}
			}
		})
	}
}

func TestOpenVaultSearchWithoutEmbedder(t *testing.T) {
	m := model{vaultSearchInput: textinput.New()}
	m.openVaultSearch()
	if m.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", m.inputMode)
	}
	if m.errMsg == "" {
		t.Error("expected a configuration error message")
	}
}

func TestToggleChat(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.toggleChat()
	if !m.chatOpen {
		t.Fatal("chat did not open")
	}
	if m.chatMode != chatModeInput {
		t.Errorf("chatMode = %v, want chatModeInput", m.chatMode)
	}
	m.toggleChat()
	if m.chatOpen {
		t.Error("chat did not close")
	}
}

func TestRejectDiffClearsRunState(t *testing.T) {
	m := model{
		inputMode:          inputPluginDiff,
		pluginDiffOriginal: "before",
		pluginDiffResult:   "after",
		aiRunOnSelection:   true,
	}
	m.rejectDiff()
	if m.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", m.inputMode)
	}
	if m.pluginDiffOriginal != "" || m.pluginDiffResult != "" || m.aiRunOnSelection {
		t.Error("run state not cleared")
	}
}

func TestAcceptDiffReplacesBuffer(t *testing.T) {
	m := model{
		inputMode:        inputPluginDiff,
		editor:           newSelectableEditor(),
		pluginDiffResult: "new content",
	}
	m.editor.SetValue("old content")
	m.acceptDiff()
	if got := m.editor.Value(); got != "new content" {
		t.Errorf("editor value = %q, want %q", got, "new content")
	}
	if m.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", m.inputMode)
	}
	if m.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", m.editorMode)
	}
}

func TestCopyReviewSetsStatus(t *testing.T) {
	m := model{inputMode: inputPluginReview, pluginDiffResult: "the review"}
	m.copyReview()
	if m.errMsg != "Review copied" {
		t.Errorf("errMsg = %q, want %q", m.errMsg, "Review copied")
	}
	if m.inputMode != inputPluginReview {
		t.Error("copy must not leave review mode")
	}
}
