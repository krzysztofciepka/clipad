package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleTemplatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.templateCursor > 0 {
			m.templateCursor--
		}
	case "down", "j":
		if m.templateCursor < len(m.templateList)-1 {
			m.templateCursor++
		}
	case "enter":
		if len(m.templateList) == 0 || m.templateCursor >= len(m.templateList) {
			m.inputMode = inputNone
			return m, nil
		}
		m.templateChosen = m.templateList[m.templateCursor]
		m.inputMode = inputTemplateName
		m.templateNameInput.SetValue("")
		cmd := m.templateNameInput.Focus()
		return m, cmd
	case "esc":
		m.inputMode = inputNone
	}
	return m, nil
}

func (m model) handleTemplateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.templateNameInput.Value())
		if name == "" {
			return m, nil
		}
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		dir := m.templateTargetDir
		if dir == "" {
			dir = m.vault
		}
		fullPath := filepath.Join(dir, name)
		if _, err := os.Stat(fullPath); err == nil {
			m.errMsg = fmt.Sprintf("File already exists: %s", name)
			return m, nil
		}
		tmpl, err := os.ReadFile(filepath.Join(templatesDir(), m.templateChosen))
		if err != nil {
			m.errMsg = fmt.Sprintf("Read template failed: %v", err)
			m.inputMode = inputNone
			return m, nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.errMsg = fmt.Sprintf("Create dir failed: %v", err)
			m.inputMode = inputNone
			return m, nil
		}
		rendered := renderTemplate(string(tmpl), time.Now(), m.vault)
		if err := os.WriteFile(fullPath, []byte(rendered), 0o644); err != nil {
			m.errMsg = fmt.Sprintf("Save failed: %v", err)
			m.inputMode = inputNone
			return m, nil
		}
		m.inputMode = inputNone
		m.openFile(fullPath)
		m.activePanel = editorPanel
		m.editor.Focus()
		m.refreshTree()
		return m, nil
	case "esc":
		m.inputMode = inputNone
		return m, nil
	}
	var cmd tea.Cmd
	m.templateNameInput, cmd = m.templateNameInput.Update(msg)
	return m, cmd
}
