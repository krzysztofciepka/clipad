package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	toml "github.com/pelletier/go-toml/v2"
)

type ConfigField struct {
	Key         string
	Label       string
	Placeholder string
	Secret      bool
}

type Plugin interface {
	Name() string
	Description() string
	ConfigFields() []ConfigField
	Run(content string, prompt string, config map[string]string) (string, error)
}

type pluginResultMsg struct {
	result string
	err    error
}

func runPluginCmd(p Plugin, content, prompt string, cfg map[string]string) tea.Cmd {
	return func() tea.Msg {
		result, err := p.Run(content, prompt, cfg)
		return pluginResultMsg{result: result, err: err}
	}
}

func pluginConfigPath(name string) string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "plugins", name+".toml")
}

func loadPluginConfig(name string) (map[string]string, error) {
	data, err := os.ReadFile(pluginConfigPath(name))
	if err != nil {
		return nil, fmt.Errorf("reading plugin config: %w", err)
	}
	var values map[string]string
	if err := toml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parsing plugin config: %w", err)
	}
	return values, nil
}

func savePluginConfig(name string, values map[string]string) error {
	path := pluginConfigPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating plugin config dir: %w", err)
	}
	data, err := toml.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshaling plugin config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func pluginConfigComplete(fields []ConfigField, values map[string]string) bool {
	for _, f := range fields {
		if values[f.Key] == "" {
			return false
		}
	}
	return true
}
