package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type setupModel struct {
	input  textinput.Model
	errMsg string
	done   bool
	vault  string
}

func newSetupModel() setupModel {
	ti := textinput.New()
	ti.Placeholder = "/home/user/notes"
	ti.CharLimit = 512
	ti.Width = 50
	ti.Focus()
	return setupModel{input: ti}
}

func (m setupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			path := m.input.Value()
			if path == "" {
				m.errMsg = "Please enter a path"
				return m, nil
			}
			if len(path) > 0 && path[0] == '~' {
				home, _ := os.UserHomeDir()
				path = home + path[1:]
			}
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				if err := os.MkdirAll(path, 0o755); err != nil {
					m.errMsg = fmt.Sprintf("Cannot create: %v", err)
					return m, nil
				}
			} else if err != nil {
				m.errMsg = fmt.Sprintf("Error: %v", err)
				return m, nil
			} else if !info.IsDir() {
				m.errMsg = "Path is not a directory"
				return m, nil
			}

			cfg := Config{Vault: path}
			if err := saveConfig(cfg); err != nil {
				m.errMsg = fmt.Sprintf("Save config failed: %v", err)
				return m, nil
			}
			m.vault = path
			m.done = true
			return m, tea.Quit

		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m setupModel) View() string {
	s := lipgloss.NewStyle().Padding(1, 2)
	title := lipgloss.NewStyle().Bold(true).Render("Welcome to Clipad!")
	prompt := "\nEnter your vault path:"
	input := "\n\n" + m.input.View()
	hint := "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		"The directory will be created if it doesn't exist. Press Enter to confirm.")
	errView := ""
	if m.errMsg != "" {
		errView = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.errMsg)
	}
	return s.Render(title + prompt + input + hint + errView)
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		setup := newSetupModel()
		p := tea.NewProgram(setup, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		sm := result.(setupModel)
		if !sm.done {
			os.Exit(0)
		}
		cfg = Config{Vault: sm.vault}
	}

	if _, err := os.Stat(cfg.Vault); err != nil {
		fmt.Fprintf(os.Stderr, "Vault directory not found: %s\n", cfg.Vault)
		os.Exit(1)
	}

	plugins := []Plugin{
		&BlackboxPlugin{},
		&OpenRouterPlugin{},
	}
	m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider)

	// Create welcome note if vault is empty (after tree was already built in newModel)
	if m.treeRoot != nil && len(collectFiles(m.treeRoot)) == 0 {
		os.WriteFile(filepath.Join(cfg.Vault, "welcome.md"),
			[]byte("# Welcome to Clipad\n\nStart writing your notes here.\n"), 0o644)
		m.refreshTree()
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
