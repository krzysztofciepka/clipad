// autosave.go
package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type autoSaveTickMsg struct{}
type autoSaveFadeMsg struct{}

func autoSaveTick() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg {
		return autoSaveTickMsg{}
	})
}

func autoSaveFadeTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return autoSaveFadeMsg{}
	})
}
