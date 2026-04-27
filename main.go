package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// version is overridden at release build time via:
//   go build -ldflags "-X main.version=vX.Y.Z" .
var version = "dev"

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
	var (
		showVersion bool
		doUpgrade   bool
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&doUpgrade, "upgrade", false, "fetch the latest release and replace this binary")
	flag.Parse()

	if showVersion {
		fmt.Printf("clipad %s\n", version)
		return
	}
	if doUpgrade {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot determine clipad binary path: %v\n", err)
			os.Exit(1)
		}
		if err := runUpgrade(os.Stderr, version, "https://api.github.com", exe); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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

	emb, embErr := newEmbeddingClient(cfg)
	var idx *Index
	if embErr == nil {
		idx, _ = OpenIndex(indexDBPath(), cfg.Vault, emb)
	}
	defer func() {
		if idx != nil {
			idx.Close()
		}
	}()

	m := newModel(cfg.Vault, plugins, cfg.AIShortcutProvider, cfg.InboxPath)
	m.indexer = idx
	if embErr != nil {
		m.errMsg = "Embeddings disabled: " + embErr.Error()
	}

	// Create welcome note if vault is empty (after tree was already built in newModel)
	if m.treeRoot != nil && len(collectFiles(m.treeRoot)) == 0 {
		os.WriteFile(filepath.Join(cfg.Vault, "welcome.md"),
			[]byte("# Welcome to Clipad\n\nStart writing your notes here.\n"), 0o644)
		m.refreshTree()
	}
	// Detect terminal background once, before tea.Program claims stdin.
	// Doing this inside glamour.WithAutoStyle() while the alt-screen is
	// active causes the OSC 11 reply to be delivered to Bubble Tea's
	// input loop as keyboard runes (visible as "]11;rgb:0000/0000/0000")
	// and freezes the first preview render until termenv times out.
	setDarkBackground(termenv.HasDarkBackground())

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
