package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleShortcutSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.shortcutCursor > 0 {
			m.shortcutCursor--
		}
	case "down", "j":
		if m.shortcutCursor < len(m.shortcuts)-1 {
			m.shortcutCursor++
		}
	case "enter":
		if len(m.shortcuts) == 0 || m.shortcutCursor >= len(m.shortcuts) {
			return m, nil
		}
		shortcut := m.shortcuts[m.shortcutCursor]
		cfg, err := loadPluginConfig("openrouter")
		if err != nil || !pluginConfigComplete((&OpenRouterPlugin{}).ConfigFields(), cfg) {
			m.shortcutPending = true
			m.pluginActive = &OpenRouterPlugin{}
			m.pluginConfigFields = m.pluginActive.ConfigFields()
			m.pluginConfigIndex = 0
			m.pluginConfigValues = make(map[string]string)
			m.inputMode = inputPluginConfig
			m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
			return m, textinput.Blink
		}
		content := m.editor.Value()
		m.shortcutOnSelection = m.editor.selActive
		if m.shortcutOnSelection {
			content = m.editor.SelectedText()
		}
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runShortcutCmd(shortcut, content, cfg)
	case "e":
		if len(m.shortcuts) > 0 && m.shortcutCursor < len(m.shortcuts) {
			m.shortcutEditing = m.shortcutCursor
			m.inputMode = inputShortcutName
			m.shortcutNameInput.SetValue(m.shortcuts[m.shortcutCursor].Name)
			cmd := m.shortcutNameInput.Focus()
			return m, cmd
		}
	case "d":
		if len(m.shortcuts) > 0 && m.shortcutCursor < len(m.shortcuts) {
			m.inputMode = inputShortcutDeleteConfirm
		}
	case "esc":
		m.inputMode = inputNone
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m model) handleShortcutName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := m.shortcutNameInput.Value()
		if name == "" {
			return m, nil
		}
		m.shortcutTempName = name
		m.inputMode = inputShortcutPrompt
		if m.shortcutEditing >= 0 {
			m.shortcutPromptInput.SetValue(m.shortcuts[m.shortcutEditing].Prompt)
		} else {
			m.shortcutPromptInput.SetValue("")
		}
		cmd := m.shortcutPromptInput.Focus()
		return m, cmd
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutNameInput, cmd = m.shortcutNameInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := m.shortcutPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		shortcut := AIShortcut{Name: m.shortcutTempName, Prompt: prompt}
		if m.shortcutEditing >= 0 {
			m.shortcuts[m.shortcutEditing] = shortcut
		} else {
			m.shortcuts = append(m.shortcuts, shortcut)
		}
		if err := saveShortcuts(m.shortcuts); err != nil {
			m.errMsg = "Failed to save shortcuts: " + err.Error()
		}
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutPromptInput, cmd = m.shortcutPromptInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		if m.shortcutCursor < len(m.shortcuts) {
			m.shortcuts = append(m.shortcuts[:m.shortcutCursor], m.shortcuts[m.shortcutCursor+1:]...)
			if err := saveShortcuts(m.shortcuts); err != nil {
				m.errMsg = "Failed to save shortcuts: " + err.Error()
			}
			if m.shortcutCursor >= len(m.shortcuts) && m.shortcutCursor > 0 {
				m.shortcutCursor--
			}
		}
		if len(m.shortcuts) == 0 {
			m.inputMode = inputNone
		} else {
			m.inputMode = inputShortcutSelect
		}
	case "n", "esc":
		m.inputMode = inputShortcutSelect
	}
	return m, nil
}
