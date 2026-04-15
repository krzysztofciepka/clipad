package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleGitRemoteInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		url := m.gitRemoteInput.Value()
		if url == "" {
			return m, nil
		}
		cfg, err := loadConfig()
		if err != nil {
			m.errMsg = "Failed to load config: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		cfg.GitRemote = url
		if err := saveConfig(cfg); err != nil {
			m.errMsg = "Failed to save config: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		m.inputMode = inputNone
		// Trigger sync immediately now that we have a remote
		return m, gitSyncCheckImmediate()
	case "esc":
		m.inputMode = inputNone
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
	m.gitRemoteInput, cmd = m.gitRemoteInput.Update(msg)
	return m, cmd
}
