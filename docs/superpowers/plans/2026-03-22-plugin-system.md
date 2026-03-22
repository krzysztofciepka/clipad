# Plugin System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a plugin system with an OpenRouter LLM plugin that processes notes via custom prompts and shows a side-by-side diff for accept/reject.

**Architecture:** Plugin interface with built-in implementations registered at startup. New input modes for plugin selector modal, config prompts, prompt input, and diff view. Async plugin execution via `tea.Cmd`. Per-plugin TOML config under `~/.config/clipad/plugins/`.

**Tech Stack:** Go, Bubble Tea, Lipgloss, go-toml/v2, net/http, encoding/json

**Spec:** `docs/superpowers/specs/2026-03-22-plugin-system-design.md`

---

## File Structure

| File | Responsibility |
|------|----------------|
| `plugin.go` | Plugin interface, ConfigField struct, config load/save helpers, `runPluginCmd`, `pluginResultMsg` |
| `plugin_test.go` | Tests for config load/save helpers |
| `plugin_openrouter.go` | OpenRouterPlugin struct implementing Plugin interface |
| `plugin_openrouter_test.go` | Tests for OpenRouter request/response parsing |
| `plugin_modal.go` | Plugin selector modal: list rendering, navigation, View helper |
| `plugin_input.go` | Prompt input + config prompt flow handlers |
| `plugin_diff.go` | Side-by-side diff view rendering with accept/reject |
| `model.go` | Modified: new input modes, Ctrl+Space, plugin state fields, pluginResultMsg in Update(), pluginProcessing guard, recalcLayout for diff viewports |
| `main.go` | Modified: pass plugins to newModel |
| `statusbar.go` | Modified: add ^Space hint when file is open |

---

### Task 1: Plugin Interface & Config Helpers

**Files:**
- Create: `plugin.go`
- Create: `plugin_test.go`

- [ ] **Step 1: Write failing tests for plugin config**

Create `plugin_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := pluginConfigPath("openrouter")
	want := "/tmp/test-xdg/clipad/plugins/openrouter.toml"
	if got != want {
		t.Errorf("pluginConfigPath() = %q, want %q", got, want)
	}
}

func TestSaveAndLoadPluginConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	values := map[string]string{
		"api_key": "sk-test-123",
		"model":   "openai/gpt-4o",
	}
	if err := savePluginConfig("testplugin", values); err != nil {
		t.Fatalf("savePluginConfig() error: %v", err)
	}

	loaded, err := loadPluginConfig("testplugin")
	if err != nil {
		t.Fatalf("loadPluginConfig() error: %v", err)
	}
	if loaded["api_key"] != "sk-test-123" {
		t.Errorf("api_key = %q, want %q", loaded["api_key"], "sk-test-123")
	}
	if loaded["model"] != "openai/gpt-4o" {
		t.Errorf("model = %q, want %q", loaded["model"], "openai/gpt-4o")
	}
}

func TestLoadPluginConfig_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	_, err := loadPluginConfig("nonexistent")
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestPluginConfigComplete(t *testing.T) {
	fields := []ConfigField{
		{Key: "api_key", Label: "API Key"},
		{Key: "model", Label: "Model"},
	}

	complete := map[string]string{"api_key": "key", "model": "m"}
	if !pluginConfigComplete(fields, complete) {
		t.Error("expected complete config to return true")
	}

	partial := map[string]string{"api_key": "key"}
	if pluginConfigComplete(fields, partial) {
		t.Error("expected partial config to return false")
	}

	empty := map[string]string{}
	if pluginConfigComplete(fields, empty) {
		t.Error("expected empty config to return false")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -v -run "TestPlugin"
```
Expected: FAIL — types and functions not defined.

- [ ] **Step 3: Implement plugin.go**

Create `plugin.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -v -run "TestPlugin"
```
Expected: PASS — all 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add plugin.go plugin_test.go
git commit -m "feat: add plugin interface, config helpers, and runPluginCmd"
```

---

### Task 2: OpenRouter Plugin

**Files:**
- Create: `plugin_openrouter.go`
- Create: `plugin_openrouter_test.go`

- [ ] **Step 1: Write failing tests**

Create `plugin_openrouter_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterPlugin_Name(t *testing.T) {
	p := &OpenRouterPlugin{}
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openrouter")
	}
}

func TestOpenRouterPlugin_ConfigFields(t *testing.T) {
	p := &OpenRouterPlugin{}
	fields := p.ConfigFields()
	if len(fields) != 2 {
		t.Fatalf("ConfigFields() returned %d fields, want 2", len(fields))
	}
	if fields[0].Key != "api_key" || !fields[0].Secret {
		t.Errorf("first field: key=%q secret=%v, want api_key/true", fields[0].Key, fields[0].Secret)
	}
	if fields[1].Key != "model" || fields[1].Secret {
		t.Errorf("second field: key=%q secret=%v, want model/false", fields[1].Key, fields[1].Secret)
	}
}

func TestOpenRouterPlugin_Run_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "openai/gpt-4o" {
			t.Errorf("model = %v, want openai/gpt-4o", req["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "Translated content"}},
			},
		})
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "test-key", "model": "openai/gpt-4o"}
	result, err := p.Run("Original content", "Translate to Polish", cfg)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result != "Translated content" {
		t.Errorf("Run() = %q, want %q", result, "Translated content")
	}
}

func TestOpenRouterPlugin_Run_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "bad-key", "model": "openai/gpt-4o"}
	_, err := p.Run("content", "prompt", cfg)
	if err == nil {
		t.Error("expected error for 401 response, got nil")
	}
}

func TestOpenRouterPlugin_Run_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	p := &OpenRouterPlugin{BaseURL: server.URL}
	cfg := map[string]string{"api_key": "key", "model": "m"}
	_, err := p.Run("content", "prompt", cfg)
	if err == nil {
		t.Error("expected error for empty choices, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -v -run "TestOpenRouter"
```
Expected: FAIL — `OpenRouterPlugin` not defined.

- [ ] **Step 3: Implement plugin_openrouter.go**

Create `plugin_openrouter.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultOpenRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type OpenRouterPlugin struct {
	BaseURL string // override for testing; empty uses default
}

func (p *OpenRouterPlugin) Name() string        { return "openrouter" }
func (p *OpenRouterPlugin) Description() string  { return "LLM-powered note transformation via OpenRouter" }

func (p *OpenRouterPlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key", Placeholder: "sk-or-...", Secret: true},
		{Key: "model", Label: "Model", Placeholder: "openai/gpt-4o", Secret: false},
	}
}

func (p *OpenRouterPlugin) Run(content string, prompt string, config map[string]string) (string, error) {
	url := p.BaseURL
	if url == "" {
		url = defaultOpenRouterURL
	}

	reqBody := map[string]interface{}{
		"model": config["model"],
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a note editor. Apply the following transformation to the note provided by the user. Return only the transformed note content, no explanations.",
			},
			{
				"role":    "user",
				"content": fmt.Sprintf("Instruction: %s\n\nNote:\n%s", prompt, content),
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config["api_key"])

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -v -run "TestOpenRouter"
```
Expected: PASS — all 4 tests green.

- [ ] **Step 5: Commit**

```bash
git add plugin_openrouter.go plugin_openrouter_test.go
git commit -m "feat: add OpenRouter plugin with API integration and tests"
```

---

### Task 3: Plugin Selector Modal

**Files:**
- Create: `plugin_modal.go`

- [ ] **Step 1: Implement plugin_modal.go**

Create `plugin_modal.go`:

```go
package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	pluginModalStyle = lipgloss.NewStyle().
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("117"))

	pluginItemStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	pluginSelectedStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	pluginDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
)

func pluginModalView(plugins []Plugin, cursor int, width, height int) string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Render("Plugins")
	b.WriteString(title)
	b.WriteString("\n\n")

	for i, p := range plugins {
		line := fmt.Sprintf("%s  %s", p.Name(), pluginDescStyle.Render(p.Description()))
		if i == cursor {
			line = pluginSelectedStyle.Render(fmt.Sprintf("> %s  %s", p.Name(), p.Description()))
		} else {
			line = pluginItemStyle.Render(line)
		}
		b.WriteString(line)
		if i < len(plugins)-1 {
			b.WriteString("\n")
		}
	}

	content := b.String()
	return pluginModalStyle.Width(width - 4).Height(height - 4).Render(content)
}
```

- [ ] **Step 2: Do NOT commit yet** — this file depends on model changes in Task 6. It will be committed together with Tasks 4, 5, and 6.

---

### Task 4: Plugin Input Handlers (Config + Prompt)

**Files:**
- Create: `plugin_input.go`

- [ ] **Step 1: Implement plugin_input.go**

Create `plugin_input.go`:

```go
package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handlePluginSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.pluginCursor > 0 {
			m.pluginCursor--
		}
	case "down", "j":
		if m.pluginCursor < len(m.plugins)-1 {
			m.pluginCursor++
		}
	case "enter":
		if m.pluginCursor < len(m.plugins) {
			m.pluginActive = m.plugins[m.pluginCursor]
			// Check config
			cfg, err := loadPluginConfig(m.pluginActive.Name())
			if err != nil || !pluginConfigComplete(m.pluginActive.ConfigFields(), cfg) {
				// Enter config flow
				m.pluginConfigFields = m.pluginActive.ConfigFields()
				m.pluginConfigIndex = 0
				m.pluginConfigValues = make(map[string]string)
				m.inputMode = inputPluginConfig
				m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
				return m, textinput.Blink
			}
			// Config exists, go to prompt
			m.inputMode = inputPluginPrompt
			m.pluginPromptInput.SetValue("")
			cmd := m.pluginPromptInput.Focus()
			return m, cmd
		}
	case "esc", "ctrl+c":
		m.inputMode = inputNone
		m.pluginActive = nil
	case "ctrl+q":
		if m.dirty {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m model) handlePluginConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		value := m.pluginConfigInput.Value()
		if value == "" {
			// Reject empty — re-prompt
			return m, nil
		}
		field := m.pluginConfigFields[m.pluginConfigIndex]
		m.pluginConfigValues[field.Key] = value
		m.pluginConfigIndex++

		if m.pluginConfigIndex >= len(m.pluginConfigFields) {
			// All fields collected — save and proceed to prompt
			if err := savePluginConfig(m.pluginActive.Name(), m.pluginConfigValues); err != nil {
				m.errMsg = "Failed to save plugin config: " + err.Error()
				m.inputMode = inputNone
				return m, nil
			}
			m.inputMode = inputPluginPrompt
			m.pluginPromptInput.SetValue("")
			cmd := m.pluginPromptInput.Focus()
			return m, cmd
		}
		// Next field
		m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[m.pluginConfigIndex])
		return m, textinput.Blink
	case "esc", "ctrl+c":
		m.inputMode = inputNone
		m.pluginActive = nil
	case "ctrl+q":
		if m.dirty {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.pluginConfigInput, cmd = m.pluginConfigInput.Update(msg)
	return m, cmd
}

func (m model) handlePluginPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		prompt := m.pluginPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		cfg, err := loadPluginConfig(m.pluginActive.Name())
		if err != nil {
			m.errMsg = "Failed to load plugin config: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		content := m.editor.Value()
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runPluginCmd(m.pluginActive, content, prompt, cfg)
	case "esc", "ctrl+c":
		m.inputMode = inputNone
		m.pluginActive = nil
	case "ctrl+q":
		if m.dirty {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.pluginPromptInput, cmd = m.pluginPromptInput.Update(msg)
	return m, cmd
}

func newPluginConfigInput(field ConfigField) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = field.Placeholder
	ti.CharLimit = 256
	if field.Secret {
		ti.EchoMode = textinput.EchoPassword
	}
	ti.Focus()
	return ti
}
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```

Note: this will fail until model.go has the new fields and input modes. This is expected — Task 6 wires everything together. For now verify there are no syntax errors by checking `go vet ./...` after Task 6.

- [ ] **Step 3: Do NOT commit yet** — depends on model changes in Task 6.

---

### Task 5: Diff View

**Files:**
- Create: `plugin_diff.go`

- [ ] **Step 1: Implement plugin_diff.go**

Create `plugin_diff.go`:

```go
package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	diffHeaderOriginal = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Padding(0, 1).
		Render("── Original ──")

	diffHeaderNew = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("76")).
		Padding(0, 1).
		Render("── New ──")

	diffBorderStyle = lipgloss.NewStyle().
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder())
)

func (m model) handlePluginDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.editor.SetValue(m.pluginDiffResult)
		m.dirty = true
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
	case "n", "esc":
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
	case "ctrl+q", "ctrl+c":
		if m.dirty {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	case "up", "k":
		m.pluginDiffViewL.LineUp(1)
		m.pluginDiffViewR.LineUp(1)
		return m, nil
	case "down", "j":
		m.pluginDiffViewL.LineDown(1)
		m.pluginDiffViewR.LineDown(1)
		return m, nil
	}
	return m, nil
}

func pluginDiffView(left, right viewport.Model, width, height int) string {
	halfWidth := width / 2

	leftPanel := diffBorderStyle.Width(halfWidth).Height(height).Render(
		diffHeaderOriginal + "\n" + left.View())

	rightPanel := lipgloss.NewStyle().Width(width - halfWidth).Height(height).Render(
		diffHeaderNew + "\n" + right.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func newDiffViewports(original, result string, width, height int) (viewport.Model, viewport.Model) {
	halfWidth := width / 2
	contentHeight := height - 1 // account for header line

	left := viewport.New(halfWidth-2, contentHeight)
	left.SetContent(original)

	right := viewport.New(width-halfWidth-2, contentHeight)
	right.SetContent(result)

	return left, right
}
```

- [ ] **Step 2: Verify build compiles**

```bash
go build ./...
```

(Same note as Task 4 — needs Task 6 to wire into model.go)

- [ ] **Step 3: Do NOT commit yet** — depends on model changes in Task 6.

---

### Task 6: Wire Plugin System into Model

**Files:**
- Modify: `model.go`
- Modify: `main.go`
- Modify: `statusbar.go`

This task integrates all plugin components into the main model.

- [ ] **Step 1: Add new input modes to model.go**

Add to the `inputMode` enum (after `inputUnsavedGuard`):

```go
	inputPluginSelect
	inputPluginConfig
	inputPluginPrompt
	inputPluginDiff
```

- [ ] **Step 2: Add plugin fields to model struct**

Add these fields to the `model` struct:

```go
	// Plugin system
	plugins            []Plugin
	pluginCursor       int
	pluginActive       Plugin
	pluginPromptInput  textinput.Model
	pluginConfigFields []ConfigField
	pluginConfigIndex  int
	pluginConfigValues map[string]string
	pluginConfigInput  textinput.Model
	pluginDiffOriginal string
	pluginDiffResult   string
	pluginDiffViewL    viewport.Model
	pluginDiffViewR    viewport.Model
	pluginProcessing   bool
```

- [ ] **Step 3: Update newModel to accept plugins**

Change `newModel` signature from `func newModel(vault string) model` to:

```go
func newModel(vault string, plugins []Plugin) model {
```

Add inside the function, in the model initialization:

```go
	pi := textinput.New()
	pi.Placeholder = "Enter prompt..."
	pi.CharLimit = 500
```

And add to the model struct literal:

```go
		plugins:           plugins,
		pluginPromptInput: pi,
```

- [ ] **Step 4: Add pluginProcessing guard and pluginResultMsg to Update()**

In the `Update()` method, add a new case in the type switch (before `tea.KeyMsg`):

```go
	case pluginResultMsg:
		m.pluginProcessing = false
		if msg.err != nil {
			m.errMsg = "Plugin error: " + msg.err.Error()
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			return m, nil
		}
		if msg.result == m.pluginDiffOriginal {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			return m, nil
		}
		m.pluginDiffResult = msg.result
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, msg.result, m.editorWidth, m.editorHeight)
		m.inputMode = inputPluginDiff
		return m, nil
```

At the top of the `tea.KeyMsg` case, add a guard for processing state:

```go
	case tea.KeyMsg:
		// Block all input while plugin is processing
		if m.pluginProcessing {
			return m, nil
		}
```

- [ ] **Step 5: Add Ctrl+Space to global keybindings**

In the global keybinding switch (inside the `tea.KeyMsg` case, after `inputMode` check), add:

```go
		case "ctrl+@":
			// Ctrl+Space — only when a file is open
			if m.currentFile != "" || m.newNoteDir != "" {
				if len(m.plugins) > 0 {
					m.inputMode = inputPluginSelect
					m.pluginCursor = 0
				}
			}
			return m, nil
```

Note: Bubble Tea represents Ctrl+Space as `"ctrl+@"`.

- [ ] **Step 6: Add plugin modes to handleInputMode()**

Add cases to `handleInputMode()`:

```go
	case inputPluginSelect:
		return m.handlePluginSelect(msg)
	case inputPluginConfig:
		return m.handlePluginConfig(msg)
	case inputPluginPrompt:
		return m.handlePluginPrompt(msg)
	case inputPluginDiff:
		return m.handlePluginDiff(msg)
```

- [ ] **Step 7: Update View() for plugin modes**

In `View()`, add plugin-specific rendering. For the right panel, add before the existing `if m.currentFile == ""` check:

```go
	if m.inputMode == inputPluginDiff {
		rightView = pluginDiffView(m.pluginDiffViewL, m.pluginDiffViewR, m.editorWidth, m.editorHeight)
	} else if m.inputMode == inputPluginSelect {
		rightView = pluginModalView(m.plugins, m.pluginCursor, m.editorWidth, m.editorHeight)
	} else if m.currentFile == "" && m.newNoteDir == "" {
```

For the status bar section, add plugin overlay cases:

```go
	if m.pluginProcessing {
		statusView = statusBarStyle.Width(m.width).Render("Processing...")
	} else if m.inputMode == inputPluginConfig {
		field := m.pluginConfigFields[m.pluginConfigIndex]
		statusView = statusBarStyle.Width(m.width).Render(
			field.Label + ": " + m.pluginConfigInput.View())
	} else if m.inputMode == inputPluginPrompt {
		statusView = statusBarStyle.Width(m.width).Render(
			"Prompt: " + m.pluginPromptInput.View())
	} else if m.inputMode == inputPluginDiff {
		statusView = statusBarStyle.Width(m.width).Render(
			"Accept changes? (y/n)")
	} else if m.inputMode == inputConfirmDelete {
```

- [ ] **Step 8: Update recalcLayout() for diff viewports**

Add at the end of `recalcLayout()`:

```go
	if m.inputMode == inputPluginDiff {
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	}
```

- [ ] **Step 9: Update main.go to pass plugins**

In `main.go`, change the `newModel` call from:

```go
	m := newModel(cfg.Vault)
```

to:

```go
	plugins := []Plugin{
		&OpenRouterPlugin{},
	}
	m := newModel(cfg.Vault, plugins)
```

- [ ] **Step 10: Update statusbar.go to show ^Space hint**

In `statusbar.go`, in the `View()` method, add after the `^P preview` line:

```go
	if s.fileOpen {
		left += "  " + statusKeyStyle.Render("^Space") + " plugins"
	}
```

Add `fileOpen bool` field to the `StatusBar` struct, and set it in `model.go`'s `View()` where the `StatusBar` is constructed:

```go
	sb := StatusBar{
		...
		fileOpen:   m.currentFile != "" || m.newNoteDir != "",
	}
```

- [ ] **Step 11: Verify build and tests**

```bash
go build -o clipad . && go test ./... && go vet ./...
```
Expected: compiles, all tests pass, no vet issues.

- [ ] **Step 12: Commit**

```bash
git add plugin_modal.go plugin_input.go plugin_diff.go model.go main.go statusbar.go
git commit -m "feat: add plugin UI components and wire into main model"
```

---

### Task 7: Manual Smoke Test & Final Verification

- [ ] **Step 1: Run all tests**

```bash
go test -v ./...
```
Expected: all tests pass.

- [ ] **Step 2: Run go vet**

```bash
go vet ./...
```
Expected: no issues.

- [ ] **Step 3: Full manual test**

Test checklist:
- [ ] `Ctrl+Space` opens plugin selector when file is open
- [ ] `Ctrl+Space` does nothing when no file is open
- [ ] Arrow keys navigate plugin list, Esc closes
- [ ] Enter on OpenRouter with no config → prompts for API Key
- [ ] Empty API key is rejected, re-prompts
- [ ] After API key, prompts for Model
- [ ] Config saved to `~/.config/clipad/plugins/openrouter.toml`
- [ ] After config, prompt input appears
- [ ] Enter with empty prompt is rejected
- [ ] "Processing..." shown during API call
- [ ] Error response shows error in status bar
- [ ] Successful response shows side-by-side diff
- [ ] Arrow keys/j,k scroll both panels
- [ ] `y` accepts changes, marks dirty
- [ ] `n` rejects, returns to editor
- [ ] Second run skips config (already saved)
- [ ] `Ctrl+Q` works in all plugin modes
- [ ] Terminal resize during diff view works

- [ ] **Step 4: Commit any fixes**

```bash
git add -A && git commit -m "fix: address issues found during manual testing"
```
(Only if changes were needed.)
