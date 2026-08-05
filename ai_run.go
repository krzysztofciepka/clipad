package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// aiRun is one invocation started from the shortcut picker — a native
// shortcut or a fabric pattern. Holding the run as a value (rather than a
// cursor into a slice) lets it survive the provider config wizard, which
// otherwise resumes against a cursor that may since have moved.
type aiRun struct {
	review bool // render the read-only review pane instead of the diff
	start  func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error)
}

// shortcutRun builds the run for a native AI shortcut.
func shortcutRun(s AIShortcut) aiRun {
	return aiRun{
		review: resolveShortcutType(s) == "review",
		start: func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error) {
			return runShortcutStream(ctx, s, content, provider, cfg)
		},
	}
}

// fabricRun builds the run for a fabric pattern. Patterns always render as a
// read-only review: most of them are analysis or extraction, so replacing the
// note with their output would destroy it.
func fabricRun(dir string, p FabricPattern) (aiRun, error) {
	system, user, err := loadFabricPattern(dir, p.Name)
	if err != nil {
		return aiRun{}, err
	}
	return aiRun{
		review: true,
		start: func(ctx context.Context, content, provider string, cfg map[string]string) (<-chan string, <-chan error) {
			return runFabricPatternStream(ctx, system, user, content, provider, cfg)
		},
	}, nil
}

// startAIRun begins streaming and switches to the diff or review view.
func (m *model) startAIRun(run aiRun, provider string, cfg map[string]string) tea.Cmd {
	content, onSelection := m.aiInputContent()
	m.aiRunOnSelection = onSelection
	m.pluginDiffOriginal = content
	m.pluginDiffResult = ""
	m.pluginProcessing = true
	ctx, cancel := context.WithCancel(context.Background())
	m.pluginCancel = cancel
	m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
	m.paneFocus = paneFocusRight
	if run.review {
		m.inputMode = inputPluginReview
		m.aiRunOnSelection = false // a review is read-only, it never replaces a selection
	} else {
		m.inputMode = inputPluginDiff
	}
	m.syncBarLayout() // the bar swaps to Approve/Reject or Copy
	chunks, errs := run.start(ctx, content, provider, cfg)
	m.activeChunks = chunks
	return streamPluginCmd(chunks, errs)
}
