package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

//go:embed defaults/ai_shortcuts.toml
var defaultShortcutsTOML []byte

type AIShortcut struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`
}

type aiShortcutsConfig struct {
	Shortcuts []AIShortcut `toml:"shortcuts"`
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

// runShortcutStream issues a streaming chat-completion using the shortcut's
// prompt as the user instruction. Returns channels that mirror Plugin.Run.
func runShortcutStream(ctx context.Context, shortcut AIShortcut, content, provider string, config map[string]string) (<-chan string, <-chan error) {
	systemPrompt := "You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."
	userMessage := fmt.Sprintf("Instruction: %s\n\nText:\n%s", shortcut.Prompt, content)
	var url string
	switch provider {
	case "openrouter":
		url = defaultOpenRouterURL
	default:
		url = defaultBlackboxURL
	}
	return streamChatCompletion(ctx, url, config["api_key"], config["model"], systemPrompt, userMessage)
}
