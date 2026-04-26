package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Vault              string     `toml:"vault"`
	GitRemote          string     `toml:"git_remote,omitempty"`
	LastSync           *time.Time `toml:"last_sync,omitempty"`
	AIShortcutProvider string     `toml:"ai_shortcut_provider,omitempty"`

	EmbeddingProvider string `toml:"embedding_provider,omitempty"`
	EmbeddingModel    string `toml:"embedding_model,omitempty"`
	OllamaURL         string `toml:"ollama_url,omitempty"`
}

const (
	defaultAIShortcutProvider       = "blackbox"
	defaultEmbeddingProvider        = "openrouter"
	defaultEmbeddingModelOpenRouter = "qwen/qwen3-embedding-8b"
	defaultEmbeddingModelOllama     = "nomic-embed-text"
	defaultOllamaURL                = "http://localhost:11434"
)

// configTOML is the on-disk representation. go-toml v2 cannot round-trip
// *time.Time (it marshals as a quoted string but then refuses to unmarshal
// that string back into *time.Time), so we store LastSync as an RFC3339
// string and convert at the boundary.
type configTOML struct {
	Vault              string `toml:"vault"`
	GitRemote          string `toml:"git_remote,omitempty"`
	LastSync           string `toml:"last_sync,omitempty"`
	AIShortcutProvider string `toml:"ai_shortcut_provider,omitempty"`

	EmbeddingProvider string `toml:"embedding_provider,omitempty"`
	EmbeddingModel    string `toml:"embedding_model,omitempty"`
	OllamaURL         string `toml:"ollama_url,omitempty"`
}

func configPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "config.toml")
}

func loadConfig() (Config, error) {
	var ct configTOML
	data, err := os.ReadFile(configPath())
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}
	if err := toml.Unmarshal(data, &ct); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	cfg := Config{
		Vault:              ct.Vault,
		GitRemote:          ct.GitRemote,
		AIShortcutProvider: ct.AIShortcutProvider,
		EmbeddingProvider:  ct.EmbeddingProvider,
		EmbeddingModel:     ct.EmbeddingModel,
		OllamaURL:          ct.OllamaURL,
	}
	if cfg.AIShortcutProvider == "" {
		cfg.AIShortcutProvider = defaultAIShortcutProvider
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = defaultEmbeddingProvider
	}
	if cfg.EmbeddingModel == "" {
		switch cfg.EmbeddingProvider {
		case "ollama":
			cfg.EmbeddingModel = defaultEmbeddingModelOllama
		default:
			cfg.EmbeddingModel = defaultEmbeddingModelOpenRouter
		}
	}
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = defaultOllamaURL
	}
	if ct.LastSync != "" {
		t, err := time.Parse(time.RFC3339, ct.LastSync)
		if err != nil {
			return Config{}, fmt.Errorf("parsing last_sync: %w", err)
		}
		cfg.LastSync = &t
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	ct := configTOML{
		Vault:              cfg.Vault,
		GitRemote:          cfg.GitRemote,
		AIShortcutProvider: cfg.AIShortcutProvider,
		EmbeddingProvider:  cfg.EmbeddingProvider,
		EmbeddingModel:     cfg.EmbeddingModel,
		OllamaURL:          cfg.OllamaURL,
	}
	if cfg.LastSync != nil {
		ct.LastSync = cfg.LastSync.Format(time.RFC3339)
	}
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := toml.Marshal(ct)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}
