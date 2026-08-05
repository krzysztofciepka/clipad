package main

import (
	"path/filepath"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// The methods in this file are the single implementation of each user-facing
// action. Both the keyboard handlers in model.go and the left-pane button bar
// call them, so the two entry points cannot drift apart.

// startFindReplace opens the find & replace prompt (Ctrl+R). No-op without an
// open note.
func (m *model) startFindReplace() tea.Cmd {
	if !m.noteOpen() {
		return nil
	}
	m.inputMode = inputReplaceSearch
	m.replaceSearchInput.SetValue("")
	m.replaceSearchTerm = ""
	return m.replaceSearchInput.Focus()
}

// openShortcutPicker opens the AI shortcut / fabric pattern picker (Ctrl+G).
// Shortcuts and patterns are re-scanned on every open so additions show up
// without restarting clipad.
func (m *model) openShortcutPicker() tea.Cmd {
	if !m.noteOpen() {
		return nil
	}
	m.shortcuts, _ = loadShortcuts()
	m.fabricPatterns = listFabricPatterns(fabricPatternsDir())
	m.shortcutFilterInput.SetValue("")
	m.inputMode = inputShortcutSelect
	m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
	m.shortcutOffset = 0
	return nil
}

// openVaultSearch opens the semantic vault search modal (Ctrl+T).
func (m *model) openVaultSearch() tea.Cmd {
	if m.indexer == nil || m.indexer.embedder == nil {
		m.errMsg = "Configure embedding_provider in config.toml"
		return nil
	}
	m.inputMode = inputVaultSearch
	m.vaultSearchInput.SetValue("")
	m.vaultSearchResults = nil
	m.vaultSearchCursor = 0
	m.vaultSearchOffset = 0
	return m.vaultSearchInput.Focus()
}

// toggleChat opens or closes the vault chat panel (Ctrl+K), cancelling any
// in-flight agent run on close.
func (m *model) toggleChat() tea.Cmd {
	if m.chatOpen {
		if m.agentCancel != nil {
			m.agentCancel()
			m.agentCancel = nil
		}
		m.chatStreaming = false
		m.agentEvents = nil
		m.chatOpen = false
		m.chatInput.Blur()
		m.recalcLayout()
		return nil
	}
	m.chatOpen = true
	m.chatMode = chatModeInput
	m.recalcLayout()
	return m.chatInput.Focus()
}

// chatFocusInput returns the vault chat panel to input mode ("i" / "/").
func (m *model) chatFocusInput() tea.Cmd {
	if !m.chatOpen {
		return nil
	}
	m.chatMode = chatModeInput
	m.buttonFocused = false
	return m.chatInput.Focus()
}

// jumpCitation steps the citation cursor by delta and opens the citation it
// lands on, exactly as the numbered 1–9 shortcuts do — including the
// unsaved-changes detour, which defers the open until the guard is answered.
func (m *model) jumpCitation(delta int) tea.Cmd {
	cites := chatCitations(m.chatTurns)
	if len(cites) == 0 {
		return nil
	}
	m.chatCiteCursor = nextCiteIndex(m.chatCiteCursor, delta, len(cites))
	c := cites[m.chatCiteCursor-1]
	abs := filepath.Join(m.vault, c.Path)
	if m.isDirty() {
		m.inputMode = inputUnsavedGuard
		m.pendingAction = pendingSwitchFile
		m.pendingSwitchPath = abs
		return nil
	}
	m.buttonFocused = false
	m.openFile(abs)
	m.editor.MoveTo(c.StartLine-1, 0)
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	return m.editor.Focus()
}

// acceptDiff applies the AI result to the buffer and leaves diff mode ("y").
func (m *model) acceptDiff() tea.Cmd {
	if m.aiRunOnSelection {
		// ReplaceSelection records its own op entry.
		m.editor.ReplaceSelection(m.pluginDiffResult)
		m.aiRunOnSelection = false
	} else {
		pre := m.editor.recordOp()
		m.editor.SetValue(m.pluginDiffResult)
		m.editor.commitOp(pre)
	}
	m.editor.ClearSelection()
	// cleanContent unchanged — editor now differs from it, so isDirty() returns true
	m.inputMode = inputNone
	m.pluginActive = nil
	m.pluginDiffOriginal = ""
	m.pluginDiffResult = ""
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	m.syncBarLayout()
	return m.editor.Focus()
}

// rejectDiff discards the AI result and leaves diff mode ("n" / Esc).
func (m *model) rejectDiff() tea.Cmd {
	m.aiRunOnSelection = false
	m.inputMode = inputNone
	m.pluginActive = nil
	m.pluginDiffOriginal = ""
	m.pluginDiffResult = ""
	m.syncBarLayout()
	return nil
}

// copyReview copies the generated review to the system clipboard ("c"). Review
// mode stays open — copying is not a dismissal.
func (m *model) copyReview() tea.Cmd {
	_ = clipboard.WriteAll(m.pluginDiffResult)
	m.errMsg = "Review copied"
	return nil
}
