package main

import (
	"context"
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
	Run(ctx context.Context, content, prompt string, cfg map[string]string) (<-chan string, <-chan error)
}

// pluginChunkMsg carries one streamed delta plus the channels needed to
// re-queue the read. The chunks field also serves as the identity token used
// to discard messages from a superseded stream.
type pluginChunkMsg struct {
	chunks <-chan string
	errs   <-chan error
	delta  string
}

// pluginDoneMsg fires when the chunks channel closes cleanly.
type pluginDoneMsg struct {
	chunks <-chan string
}

// pluginErrMsg fires when the errs channel yields a non-nil error.
type pluginErrMsg struct {
	chunks <-chan string
	err    error
}

// streamPluginCmd is the entry point: it returns the first read.
func streamPluginCmd(chunks <-chan string, errs <-chan error) tea.Cmd {
	return readNextChunk(chunks, errs)
}

// readNextChunk reads one value from chunks (or an error from errs) and
// returns a tea.Msg. The model handler re-invokes readNextChunk to keep
// draining the stream.
func readNextChunk(chunks <-chan string, errs <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case d, ok := <-chunks:
			if !ok {
				return pluginDoneMsg{chunks: chunks}
			}
			return pluginChunkMsg{chunks: chunks, errs: errs, delta: d}
		case err := <-errs:
			if err != nil {
				return pluginErrMsg{chunks: chunks, err: err}
			}
			return pluginDoneMsg{chunks: chunks}
		}
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
