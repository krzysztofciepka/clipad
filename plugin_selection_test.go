package main

import (
	"testing"
)

func TestAIInputContent_NoSelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	content, onSelection := m.aiInputContent()
	if content != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
	if onSelection {
		t.Error("onSelection = true, want false")
	}
}

func TestAIInputContent_WithSelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	content, onSelection := m.aiInputContent()
	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	if !onSelection {
		t.Error("onSelection = false, want true")
	}
}

type fakePlugin struct {
	name string
}

func (f *fakePlugin) Name() string        { return f.name }
func (f *fakePlugin) Description() string { return "fake" }
func (f *fakePlugin) ConfigFields() []ConfigField {
	return []ConfigField{
		{Key: "api_key", Label: "API Key"},
		{Key: "model", Label: "Model"},
	}
}
func (f *fakePlugin) Run(content, prompt string, config map[string]string) (string, error) {
	return "result", nil
}

func TestShortcutSelect_WithSelection_SendsOnlySelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	provider := defaultAIShortcutProvider
	plugin := pluginByName(m.plugins, provider)
	if plugin == nil {
		// newTestModel passes nil plugins; install a fake one keyed by the
		// default provider so handleShortcutSelect's lookup succeeds.
		plugin = &fakePlugin{name: provider}
		m.plugins = []Plugin{plugin}
	}
	if err := savePluginConfig(provider, map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.shortcuts = []AIShortcut{{Name: "n", Description: "d", Prompt: "p"}}
	m.shortcutCursor = 0
	m.activeShortcutProvider = provider
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(pressEnter())
	nm := next.(model)

	if nm.pluginDiffOriginal != "hello" {
		t.Errorf("pluginDiffOriginal = %q, want %q", nm.pluginDiffOriginal, "hello")
	}
	if !nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = false, want true")
	}
}

func TestPluginPrompt_NoSelection_SendsWholeContent(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	plugin := &fakePlugin{name: "fake"}
	m.pluginActive = plugin
	if err := savePluginConfig(plugin.Name(), map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.pluginPromptInput.SetValue("rewrite please")
	m.inputMode = inputPluginPrompt

	next, _ := m.handlePluginPrompt(pressEnter())
	nm := next.(model)

	if nm.pluginDiffOriginal != "hello world" {
		t.Errorf("pluginDiffOriginal = %q, want %q", nm.pluginDiffOriginal, "hello world")
	}
	if nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = true, want false")
	}
}

func TestPluginPrompt_WithSelection_SendsOnlySelection(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")
	m.editor.moveTo(0, 0)
	m.editor.selAnchorLine, m.editor.selAnchorCol, m.editor.selActive = 0, 0, true
	m.editor.moveTo(0, 5)

	plugin := &fakePlugin{name: "fake"}
	m.pluginActive = plugin
	if err := savePluginConfig(plugin.Name(), map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.pluginPromptInput.SetValue("rewrite please")
	m.inputMode = inputPluginPrompt

	next, _ := m.handlePluginPrompt(pressEnter())
	nm := next.(model)

	if nm.pluginDiffOriginal != "hello" {
		t.Errorf("pluginDiffOriginal = %q, want %q", nm.pluginDiffOriginal, "hello")
	}
	if !nm.aiRunOnSelection {
		t.Error("aiRunOnSelection = false, want true")
	}
}
