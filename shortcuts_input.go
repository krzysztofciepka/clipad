package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleShortcutSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.shortcutRows()
	visible := visibleShortcutRows(m.editorHeight)
	switch msg.String() {
	case "up", "k":
		m.shortcutCursor = nextSelectableRow(rows, m.shortcutCursor, -1)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visible)
	case "down", "j":
		m.shortcutCursor = nextSelectableRow(rows, m.shortcutCursor, 1)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visible)
	case "ctrl+up", "ctrl+down":
		if m.shortcutFilterInput.Value() != "" {
			m.errMsg = "Clear the filter to reorder shortcuts"
			return m, nil
		}
		i := m.selectedShortcutIndex()
		if i < 0 {
			m.errMsg = "Fabric patterns are read-only"
			return m, nil
		}
		j := i - 1
		if msg.String() == "ctrl+down" {
			j = i + 1
		}
		if j < 0 || j >= len(m.shortcuts) {
			return m, nil
		}
		m.shortcuts[i], m.shortcuts[j] = m.shortcuts[j], m.shortcuts[i]
		m.shortcutCursor += j - i
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visible)
		if err := saveShortcuts(m.shortcuts); err != nil {
			m.errMsg = "Failed to save shortcuts: " + err.Error()
		}
	case "enter":
		row := selectedRow(rows, m.shortcutCursor)
		var run aiRun
		switch row.kind {
		case rowShortcut:
			run = shortcutRun(m.shortcuts[row.index])
		case rowFabric:
			var err error
			run, err = fabricRun(fabricPatternsDir(), m.fabricPatterns[row.index])
			if err != nil {
				m.errMsg = "Failed to load fabric pattern: " + err.Error()
				return m, nil
			}
		default:
			return m, nil
		}
		provider := m.activeShortcutProvider
		if provider == "" {
			provider = defaultAIShortcutProvider
		}
		plugin := pluginByName(m.plugins, provider)
		if plugin == nil {
			m.errMsg = "Unknown AI shortcut provider: " + provider
			return m, nil
		}
		cfg, err := loadPluginConfig(provider)
		if err != nil || !pluginConfigComplete(plugin.ConfigFields(), cfg) {
			m.pendingAIRun = &run
			m.pluginActive = plugin
			m.pluginConfigFields = plugin.ConfigFields()
			m.pluginConfigIndex = 0
			m.pluginConfigValues = make(map[string]string)
			m.inputMode = inputPluginConfig
			m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
			return m, textinput.Blink
		}
		return m, m.startAIRun(run, provider, cfg)
	case "/":
		m.inputMode = inputShortcutFilter
		m.shortcutFilterInput.SetValue("")
		m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
		m.shortcutOffset = 0
		return m, m.shortcutFilterInput.Focus()
	case "p":
		if len(m.plugins) <= 1 {
			return m, nil
		}
		allNames := make([]string, 0, len(m.plugins))
		for _, p := range m.plugins {
			allNames = append(allNames, p.Name())
		}
		next := cycleShortcutProvider(m.activeShortcutProvider, allNames)
		if next != m.activeShortcutProvider {
			m.activeShortcutProvider = next
			if cfg, err := loadConfig(); err == nil {
				cfg.AIShortcutProvider = next
				_ = saveConfig(cfg)
			}
		}
	case "e":
		i := m.selectedShortcutIndex()
		if i < 0 {
			m.errMsg = "Fabric patterns are read-only"
			return m, nil
		}
		m.shortcutEditing = i
		m.inputMode = inputShortcutName
		m.shortcutNameInput.SetValue(m.shortcuts[i].Name)
		cmd := m.shortcutNameInput.Focus()
		return m, cmd
	case "d":
		if m.selectedShortcutIndex() < 0 {
			m.errMsg = "Fabric patterns are read-only"
			return m, nil
		}
		m.inputMode = inputShortcutDeleteConfirm
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

// handleShortcutFilter runs the picker's '/' filter, mirroring the file
// tree's inputFilter: runes narrow the list, arrows navigate, Enter runs the
// selection, Esc clears the filter and returns to the plain picker.
func (m model) handleShortcutFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.inputMode = inputShortcutSelect
		return m.handleShortcutSelect(msg)
	case "esc":
		m.shortcutFilterInput.SetValue("")
		m.shortcutFilterInput.Blur()
		m.inputMode = inputShortcutSelect
		m.shortcutCursor = clampSelectableRow(m.shortcutRows(), m.shortcutCursor)
		m.shortcutOffset = 0
		return m, nil
	case "up", "down":
		delta := 1
		if msg.String() == "up" {
			delta = -1
		}
		rows := m.shortcutRows()
		m.shortcutCursor = nextSelectableRow(rows, m.shortcutCursor, delta)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visibleShortcutRows(m.editorHeight))
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.shortcutFilterInput, cmd = m.shortcutFilterInput.Update(msg)
	m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
	m.shortcutOffset = 0
	return m, cmd
}

func (m model) handleShortcutName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := m.shortcutNameInput.Value()
		if name == "" {
			return m, nil
		}
		m.shortcutTempName = name
		m.inputMode = inputShortcutDescription
		if m.shortcutEditing >= 0 {
			m.shortcutDescriptionInput.SetValue(m.shortcuts[m.shortcutEditing].Description)
		} else {
			m.shortcutDescriptionInput.SetValue("")
		}
		cmd := m.shortcutDescriptionInput.Focus()
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

func (m model) handleShortcutDescription(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		desc := m.shortcutDescriptionInput.Value()
		if desc == "" {
			return m, nil
		}
		m.shortcutTempDescription = desc
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
	m.shortcutDescriptionInput, cmd = m.shortcutDescriptionInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := m.shortcutPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		m.shortcutTempPrompt = prompt
		if m.shortcutEditing >= 0 {
			m.shortcutTypeCursor = shortcutTypeIndex(resolveShortcutType(m.shortcuts[m.shortcutEditing]))
		} else {
			m.shortcutTypeCursor = shortcutTypeIndex("replace")
		}
		m.inputMode = inputShortcutType
		return m, nil
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
		if i := m.selectedShortcutIndex(); i >= 0 {
			m.shortcuts = append(m.shortcuts[:i], m.shortcuts[i+1:]...)
			if err := saveShortcuts(m.shortcuts); err != nil {
				m.errMsg = "Failed to save shortcuts: " + err.Error()
			}
		}
		rows := m.shortcutRows()
		m.shortcutCursor = clampSelectableRow(rows, m.shortcutCursor)
		m.shortcutOffset = clampShortcutOffset(m.shortcutCursor, m.shortcutOffset, visibleShortcutRows(m.editorHeight))
		if len(rows) == 0 {
			m.inputMode = inputNone
		} else {
			m.inputMode = inputShortcutSelect
		}
	case "n", "esc":
		m.inputMode = inputShortcutSelect
	}
	return m, nil
}
