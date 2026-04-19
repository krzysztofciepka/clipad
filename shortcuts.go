package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	toml "github.com/pelletier/go-toml/v2"
)

//go:embed defaults/ai_shortcuts.toml
var defaultShortcutsTOML []byte

type AIShortcut struct {
	Name   string `toml:"name"`
	Prompt string `toml:"prompt"`
}

type aiShortcutsConfig struct {
	Shortcuts []AIShortcut `toml:"shortcuts"`
}

type shortcutResultMsg struct {
	result string
	err    error
}

func shortcutsPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "ai_shortcuts.toml")
}

func loadShortcuts() ([]AIShortcut, error) {
	path := shortcutsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("creating shortcuts dir: %w", err)
			}
			if err := os.WriteFile(path, defaultShortcutsTOML, 0o644); err != nil {
				return nil, fmt.Errorf("seeding shortcuts: %w", err)
			}
			data = defaultShortcutsTOML
		} else {
			return nil, fmt.Errorf("reading shortcuts: %w", err)
		}
	}
	var cfg aiShortcutsConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing shortcuts: %w", err)
	}
	return cfg.Shortcuts, nil
}

func saveShortcuts(shortcuts []AIShortcut) error {
	path := shortcutsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating shortcuts dir: %w", err)
	}
	cfg := aiShortcutsConfig{Shortcuts: shortcuts}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling shortcuts: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func runShortcutCmd(shortcut AIShortcut, content, provider string, config map[string]string) tea.Cmd {
	return func() tea.Msg {
		systemPrompt := "You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."
		userMessage := fmt.Sprintf("Instruction: %s\n\nText:\n%s", shortcut.Prompt, content)
		var result string
		var err error
		switch provider {
		case "openrouter":
			result, err = callOpenRouter(defaultOpenRouterURL, config["api_key"], config["model"], systemPrompt, userMessage)
		default:
			result, err = callBlackbox(defaultBlackboxURL, config["api_key"], config["model"], systemPrompt, userMessage)
		}
		return shortcutResultMsg{result: result, err: err}
	}
}
