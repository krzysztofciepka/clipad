package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handlePluginSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.pluginCursor > 0 {
			m.pluginCursor--
		}
	case "down", "j":
		if m.pluginCursor < len(m.plugins)-1 {
			m.pluginCursor++
		}
	case "enter":
		if m.pluginCursor < len(m.plugins) {
			m.pluginActive = m.plugins[m.pluginCursor]
			cfg, err := loadPluginConfig(m.pluginActive.Name())
			if err != nil || !pluginConfigComplete(m.pluginActive.ConfigFields(), cfg) {
				m.pluginConfigFields = m.pluginActive.ConfigFields()
				m.pluginConfigIndex = 0
				m.pluginConfigValues = make(map[string]string)
				m.inputMode = inputPluginConfig
				m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
				return m, textinput.Blink
			}
			m.inputMode = inputPluginPrompt
			m.pluginPromptInput.SetValue("")
			cmd := m.pluginPromptInput.Focus()
			return m, cmd
		}
	case "esc":
		m.inputMode = inputNone
		m.pluginActive = nil
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

func (m model) handlePluginConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		value := m.pluginConfigInput.Value()
		if value == "" {
			return m, nil
		}
		field := m.pluginConfigFields[m.pluginConfigIndex]
		m.pluginConfigValues[field.Key] = value
		m.pluginConfigIndex++

		if m.pluginConfigIndex >= len(m.pluginConfigFields) {
			if err := savePluginConfig(m.pluginActive.Name(), m.pluginConfigValues); err != nil {
				m.errMsg = "Failed to save plugin config: " + err.Error()
				m.inputMode = inputNone
				m.shortcutPending = false
				return m, nil
			}
			// If config was triggered from a shortcut, execute the pending shortcut
			if m.shortcutPending {
				m.shortcutPending = false
				shortcut := m.shortcuts[m.shortcutCursor]
				provider := m.pluginActive.Name()
				cfg, _ := loadPluginConfig(provider)
				content := m.editor.Value()
				m.shortcutOnSelection = m.editor.selActive
				if m.shortcutOnSelection {
					content = m.editor.SelectedText()
				}
				m.pluginDiffOriginal = content
				m.pluginProcessing = true
				m.inputMode = inputNone
				return m, runShortcutCmd(shortcut, content, provider, cfg)
			}
			m.inputMode = inputPluginPrompt
			m.pluginPromptInput.SetValue("")
			cmd := m.pluginPromptInput.Focus()
			return m, cmd
		}
		m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[m.pluginConfigIndex])
		return m, textinput.Blink
	case "esc":
		m.inputMode = inputNone
		m.pluginActive = nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.pluginConfigInput, cmd = m.pluginConfigInput.Update(msg)
	return m, cmd
}

func (m model) handlePluginPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := m.pluginPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		cfg, err := loadPluginConfig(m.pluginActive.Name())
		if err != nil {
			m.errMsg = "Failed to load plugin config: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		content := m.editor.Value()
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
	case "esc":
		m.inputMode = inputNone
		m.pluginActive = nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.pluginPromptInput, cmd = m.pluginPromptInput.Update(msg)
	return m, cmd
}

func newPluginConfigInput(field ConfigField) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = field.Placeholder
	ti.CharLimit = 256
	if field.Secret {
		ti.EchoMode = textinput.EchoPassword
	}
	ti.Focus()
	return ti
}
