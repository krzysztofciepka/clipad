package main

import (
	"context"
	"testing"
)

func closedStream() (<-chan string, <-chan error) {
	chunks := make(chan string)
	errs := make(chan error)
	close(chunks)
	close(errs)
	return chunks, errs
}

func TestStartAIRun_ReviewOpensReadOnlyPane(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")

	run := aiRun{review: true, start: func(context.Context, string, string, map[string]string) (<-chan string, <-chan error) {
		return closedStream()
	}}
	m.startAIRun(run, "openrouter", map[string]string{"api_key": "k", "model": "m"})

	if m.inputMode != inputPluginReview {
		t.Errorf("inputMode = %v, want inputPluginReview", m.inputMode)
	}
	if m.aiRunOnSelection {
		t.Error("aiRunOnSelection = true; a review must never edit the note")
	}
	if !m.pluginProcessing {
		t.Error("pluginProcessing = false, want true")
	}
}

func TestStartAIRun_ReplaceOpensDiff(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")

	run := aiRun{review: false, start: func(context.Context, string, string, map[string]string) (<-chan string, <-chan error) {
		return closedStream()
	}}
	m.startAIRun(run, "openrouter", map[string]string{"api_key": "k", "model": "m"})

	if m.inputMode != inputPluginDiff {
		t.Errorf("inputMode = %v, want inputPluginDiff", m.inputMode)
	}
	if m.pluginDiffOriginal != "note body" {
		t.Errorf("pluginDiffOriginal = %q, want %q", m.pluginDiffOriginal, "note body")
	}
}

func TestShortcutRun_CarriesResolvedType(t *testing.T) {
	if got := shortcutRun(AIShortcut{Name: "critique", Type: "review"}); !got.review {
		t.Error("review-type shortcut produced a non-review run")
	}
	if got := shortcutRun(AIShortcut{Name: "tldr", Type: "replace"}); got.review {
		t.Error("replace-type shortcut produced a review run")
	}
	// Type inferred from the name when unset, as resolveShortcutType does.
	if got := shortcutRun(AIShortcut{Name: "questions"}); !got.review {
		t.Error("untyped 'questions' shortcut should infer review")
	}
}

func TestPluginConfigCompletion_ResumesPendingReviewRun(t *testing.T) {
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("note body")
	plugin := &fakePlugin{name: "openrouter"}
	m.plugins = []Plugin{plugin}
	m.pluginActive = plugin
	m.pluginConfigFields = plugin.ConfigFields()
	m.pluginConfigIndex = len(m.pluginConfigFields) - 1
	m.pluginConfigValues = map[string]string{"api_key": "k"}
	m.pluginConfigInput.SetValue("some-model")
	m.inputMode = inputPluginConfig
	run := aiRun{review: true, start: func(context.Context, string, string, map[string]string) (<-chan string, <-chan error) {
		return closedStream()
	}}
	m.pendingAIRun = &run

	next, _ := m.handlePluginConfig(pressEnter())
	nm := next.(model)

	if nm.inputMode != inputPluginReview {
		t.Errorf("inputMode = %v, want inputPluginReview — a pending review run must not open the diff", nm.inputMode)
	}
	if nm.pendingAIRun != nil {
		t.Error("pendingAIRun still set after resume")
	}
}
