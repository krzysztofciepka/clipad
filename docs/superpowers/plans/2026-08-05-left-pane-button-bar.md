# Left-Pane Button Bar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a persistent, clickable, keyboard-focusable button bar at the bottom of clipad's left pane that exposes the actions currently hidden behind keyboard shortcuts, with mode-specific sets for diff, review, and vault-chat modes and a one-row collapsed state.

**Architecture:** A new `buttons.go` owns the bar's data model (which buttons for which mode), height math, rendering, hit-testing, and focus keys. A new `actions.go` holds the action bodies extracted out of the `Update` key switch so the keyboard and the buttons run identical code. `model.go` gains three pieces of state, a bar-aware `recalcLayout`, and routing.

**Tech Stack:** Go, Bubble Tea (`github.com/charmbracelet/bubbletea`), Lipgloss, standard `testing` with table tests.

**Spec:** `docs/superpowers/specs/2026-08-05-left-pane-button-bar-design.md`

## Global Constraints

- Package is `main`; every file lives in the repo root next to its `_test.go` sibling. No subpackages.
- `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -l .` (no output) must pass at the end of every task. The Go code blocks in this plan are written for readability; run `gofmt -w .` after pasting them so struct literals and field alignment match the repo.
- Tests are table-driven with a `tests := []struct{...}` slice and `t.Run(tt.name, ...)`, matching `mouse_test.go` and `tree_test.go`.
- **No git operations.** The user handles all commits, branches, and releases manually. Do not run `git add`, `git commit`, `git push`, or `gh release`. There is unrelated uncommitted work in the tree.
- Keyboard behavior that exists today must not change. Every extracted action must be a pure move of the existing body.
- Bar labels are exactly: `Find & replace`, `AI tools`, `Ask`, `AI search`, `Approve`, `Reject`, `Copy`, `Input`, `Prev cite`, `Next cite`.

---

### Task 1: Bar data model and height math

Pure functions and state, no wiring. Nothing renders yet.

**Files:**
- Create: `buttons.go`
- Create: `buttons_test.go`
- Modify: `model.go:83-225` (the `model` struct)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type barAction int` with constants `actionToggleBar`, `actionFindReplace`, `actionAITools`, `actionAsk`, `actionAISearch`, `actionApprove`, `actionReject`, `actionCopyReview`, `actionChatInput`, `actionPrevCite`, `actionNextCite`
  - `type barButton struct { label string; action barAction; enabled bool }`
  - `func (m model) barButtons() []barButton`
  - `func (m model) barInert() bool`
  - `func buttonBarHeight(treeWidth, availHeight int, collapsed bool, n int) int`
  - model fields `buttonsCollapsed bool`, `buttonFocused bool`, `buttonCursor int`, `barHeight int`, `chatCiteCursor int`

- [ ] **Step 1: Write the failing tests**

Create `buttons_test.go`:

```go
package main

import "testing"

// newBarTestModel builds a fully initialised model the way the app does.
// recalcLayout touches the tree, the editor and the image registry, so tests
// that call it need real state rather than a zero-value struct. The env vars
// keep loadShortcuts away from the developer's real config, matching
// tree_toggle_test.go.
func newBarTestModel(t *testing.T, width, height int) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	m := newModel(t.TempDir(), nil, "", "")
	m.width = width
	m.height = height
	m.recalcLayout()
	return m
}

func TestButtonBarHeight(t *testing.T) {
	tests := []struct {
		name                   string
		treeWidth, availHeight int
		collapsed              bool
		n                      int
		want                   int
	}{
		{"expanded four buttons", 20, 30, false, 4, 5},
		{"collapsed", 20, 30, true, 4, 1},
		{"tree hidden", 0, 30, false, 4, 0},
		{"negative tree width", -1, 30, false, 4, 0},
		{"no room at all", 20, 1, false, 4, 0},
		{"squeezed falls back to rule row", 20, 5, false, 4, 1},
		{"exactly fits", 20, 6, false, 4, 5},
		{"collapsed still fits tiny", 20, 2, true, 4, 1},
		{"two buttons", 20, 30, false, 2, 3},
		{"one button", 20, 30, false, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buttonBarHeight(tt.treeWidth, tt.availHeight, tt.collapsed, tt.n)
			if got != tt.want {
				t.Errorf("buttonBarHeight(%d,%d,%v,%d) = %d, want %d",
					tt.treeWidth, tt.availHeight, tt.collapsed, tt.n, got, tt.want)
			}
		})
	}
}

func TestBarButtons(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(m *model)
		wantLabels []string
	}{
		{
			name:       "default set",
			setup:      func(m *model) { m.currentFile = "/v/a.md" },
			wantLabels: []string{"Find & replace", "AI tools", "Ask", "AI search"},
		},
		{
			name:       "diff mode",
			setup:      func(m *model) { m.inputMode = inputPluginDiff },
			wantLabels: []string{"Approve", "Reject"},
		},
		{
			name:       "review mode",
			setup:      func(m *model) { m.inputMode = inputPluginReview },
			wantLabels: []string{"Copy"},
		},
		{
			name:       "chat mode",
			setup:      func(m *model) { m.chatOpen = true },
			wantLabels: []string{"Input", "Prev cite", "Next cite"},
		},
		{
			name: "diff wins over chat",
			setup: func(m *model) {
				m.chatOpen = true
				m.inputMode = inputPluginDiff
			},
			wantLabels: []string{"Approve", "Reject"},
		},
		{
			name:       "modal overlay keeps the default set",
			setup:      func(m *model) { m.inputMode = inputVaultSearch },
			wantLabels: []string{"Find & replace", "AI tools", "Ask", "AI search"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{}
			tt.setup(&m)
			got := m.barButtons()
			if len(got) != len(tt.wantLabels) {
				t.Fatalf("got %d buttons, want %d: %v", len(got), len(tt.wantLabels), got)
			}
			for i, want := range tt.wantLabels {
				if got[i].label != want {
					t.Errorf("button %d = %q, want %q", i, got[i].label, want)
				}
			}
		})
	}
}

func TestBarButtonsEnablement(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(m *model)
		label   string
		enabled bool
	}{
		{"find & replace needs an open note", func(m *model) {}, "Find & replace", false},
		{"find & replace with open file", func(m *model) { m.currentFile = "/v/a.md" }, "Find & replace", true},
		{"find & replace with new note", func(m *model) { m.newNoteDir = "/v" }, "Find & replace", true},
		{"ai tools needs an open note", func(m *model) {}, "AI tools", false},
		{"ai tools with open file", func(m *model) { m.currentFile = "/v/a.md" }, "AI tools", true},
		{"ask is always live", func(m *model) {}, "Ask", true},
		{"ai search is always live", func(m *model) {}, "AI search", true},
		{
			"citation buttons need citations",
			func(m *model) { m.chatOpen = true },
			"Next cite", false,
		},
		{
			"citation buttons live once cited",
			func(m *model) {
				m.chatOpen = true
				m.chatTurns = []chatTurn{{Role: "assistant", Citations: []citation{{Path: "a.md", StartLine: 1, EndLine: 2}}}}
			},
			"Next cite", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{}
			tt.setup(&m)
			for _, b := range m.barButtons() {
				if b.label == tt.label {
					if b.enabled != tt.enabled {
						t.Errorf("%q enabled = %v, want %v", tt.label, b.enabled, tt.enabled)
					}
					return
				}
			}
			t.Fatalf("button %q not present", tt.label)
		})
	}
}

func TestBarInert(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *model)
		want  bool
	}{
		{"normal", func(m *model) {}, false},
		{"diff", func(m *model) { m.inputMode = inputPluginDiff }, false},
		{"review", func(m *model) { m.inputMode = inputPluginReview }, false},
		{"vault search modal", func(m *model) { m.inputMode = inputVaultSearch }, true},
		{"shortcut picker", func(m *model) { m.inputMode = inputShortcutSelect }, true},
		{"tree filter", func(m *model) { m.inputMode = inputFilter }, true},
		{"streaming plugin run", func(m *model) { m.pluginProcessing = true }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{}
			tt.setup(&m)
			if got := m.barInert(); got != tt.want {
				t.Errorf("barInert() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestButtonBar|TestBarButtons|TestBarInert' -v`
Expected: compile failure — `undefined: buttonBarHeight`, `undefined: barButton`, etc.

- [ ] **Step 3: Add the model state fields**

In `model.go`, inside the `model` struct, immediately after the `treeHidden` field (around line 96):

```go
	// Left-pane button bar (below the file tree)
	buttonsCollapsed bool // minified to the toggle rule row; per-session
	buttonFocused    bool // keyboard focus is on the bar
	buttonCursor     int  // 0 = toggle rule row, 1..N = buttons
	barHeight        int  // rows the bar occupies; 0 when there is no left pane
```

And in the "Agent chat panel" block, after `chatStreaming bool` (around line 203):

```go
	chatCiteCursor int // 1-based cursor into the newest cited turn; 0 = none yet
```

- [ ] **Step 4: Write `buttons.go`**

```go
package main

// barAction identifies what a button bar row does. The row→action mapping is
// data rather than closures so it stays comparable in tests.
type barAction int

const (
	actionToggleBar barAction = iota
	actionFindReplace
	actionAITools
	actionAsk
	actionAISearch
	actionApprove
	actionReject
	actionCopyReview
	actionChatInput
	actionPrevCite
	actionNextCite
)

// barButton is one clickable row of the bar. A disabled button renders dim and
// ignores clicks and Enter; it is used for actions whose guard would make them
// a silent no-op anyway.
type barButton struct {
	label   string
	action  barAction
	enabled bool
}

// noteOpen reports whether there is a buffer to act on — an existing file or an
// unsaved new note. Several actions are meaningless without one.
func (m model) noteOpen() bool {
	return m.currentFile != "" || m.newNoteDir != ""
}

// barButtons returns the button set for the current mode. Diff and review are
// checked before the chat panel: a plugin run started from the chat panel puts
// the diff on screen, and approve/reject is what matters then.
func (m model) barButtons() []barButton {
	switch m.inputMode {
	case inputPluginDiff:
		return []barButton{
			{label: "Approve", action: actionApprove, enabled: true},
			{label: "Reject", action: actionReject, enabled: true},
		}
	case inputPluginReview:
		return []barButton{
			{label: "Copy", action: actionCopyReview, enabled: true},
		}
	}
	if m.chatOpen {
		cited := len(chatCitations(m.chatTurns)) > 0
		return []barButton{
			{label: "Input", action: actionChatInput, enabled: true},
			{label: "Prev cite", action: actionPrevCite, enabled: cited},
			{label: "Next cite", action: actionNextCite, enabled: cited},
		}
	}
	open := m.noteOpen()
	return []barButton{
		{label: "Find & replace", action: actionFindReplace, enabled: open},
		{label: "AI tools", action: actionAITools, enabled: open},
		// Ask and AI search stay live without an open note: Ask works against
		// the whole vault, and AI search reports its own configuration error.
		{label: "Ask", action: actionAsk, enabled: true},
		{label: "AI search", action: actionAISearch, enabled: true},
	}
}

// barInert reports whether the bar is currently non-interactive. A modal
// overlay other than the diff and review views owns the keyboard and already
// swallows mouse events, and a streaming plugin run must not be accepted
// halfway. The bar renders dim in those states rather than looking live.
func (m model) barInert() bool {
	if m.pluginProcessing {
		return true
	}
	switch m.inputMode {
	case inputNone, inputPluginDiff, inputPluginReview:
		return false
	}
	return true
}

// buttonBarHeight returns how many rows the bar occupies. availHeight is the
// height of the left column (terminal height minus the status bar). The bar
// falls back to its single toggle row when the expanded form would leave the
// file tree no room at all.
func buttonBarHeight(treeWidth, availHeight int, collapsed bool, n int) int {
	if treeWidth <= 0 || availHeight < 2 {
		return 0
	}
	if collapsed {
		return 1
	}
	h := 1 + n
	if h > availHeight-1 {
		return 1
	}
	return h
}
```

- [ ] **Step 5: Add the `chatCitations` helper**

`barButtons` needs it now; Task 3 builds the rest of the citation logic on it. Add to `chat.go`, replacing the body of `mostRecentCitation` so there is one definition of "the current citation list":

```go
// chatCitations returns the citations of the most recent assistant turn that
// has any. The numbered 1–9 shortcuts and the Prev/Next cite buttons both
// index into this list.
func chatCitations(turns []chatTurn) []citation {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "assistant" && len(turns[i].Citations) > 0 {
			return turns[i].Citations
		}
	}
	return nil
}

// mostRecentCitation returns the Nth citation (1-indexed) of the most recent
// assistant turn that has citations, or nil if there isn't one.
func mostRecentCitation(turns []chatTurn, n int) *citation {
	cites := chatCitations(turns)
	if n < 1 || n > len(cites) {
		return nil
	}
	c := cites[n-1]
	return &c
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./... -run 'TestButtonBar|TestBarButtons|TestBarInert|TestMostRecentCitation' -v`
Expected: PASS

- [ ] **Step 7: Verify nothing else broke**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS

---

### Task 2: Extract shared action methods

Behavior-preserving refactor. The keyboard must behave identically afterwards; the buttons will call the same methods in Task 5.

**Files:**
- Create: `actions.go`
- Create: `actions_test.go`
- Modify: `model.go:834-842` (`ctrl+r`), `model.go:872-883` (`ctrl+g`), `model.go:902-913` (`ctrl+t`), `model.go:915-932` (`ctrl+k`), `model.go:412-418` (`closePluginRun`)
- Modify: `plugin_diff.go:25-53` (`y` / `n` cases)
- Modify: `plugin_review.go:142-159` (`c` case)

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces:
  - `func (m *model) startFindReplace() tea.Cmd`
  - `func (m *model) openShortcutPicker() tea.Cmd`
  - `func (m *model) openVaultSearch() tea.Cmd`
  - `func (m *model) toggleChat() tea.Cmd`
  - `func (m *model) acceptDiff() tea.Cmd`
  - `func (m *model) rejectDiff() tea.Cmd`
  - `func (m *model) copyReview() tea.Cmd`

- [ ] **Step 1: Write the failing tests**

Create `actions_test.go`:

```go
package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestStartFindReplace(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(m *model)
		wantMode  inputMode
	}{
		{"no note open is a no-op", func(m *model) {}, inputNone},
		{"open file enters replace search", func(m *model) { m.currentFile = "/v/a.md" }, inputReplaceSearch},
		{"new note enters replace search", func(m *model) { m.newNoteDir = "/v" }, inputReplaceSearch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{replaceSearchInput: textinput.New()}
			tt.setup(&m)
			m.replaceSearchInput.SetValue("stale")
			m.replaceSearchTerm = "stale"
			m.startFindReplace()
			if m.inputMode != tt.wantMode {
				t.Fatalf("inputMode = %v, want %v", m.inputMode, tt.wantMode)
			}
			if tt.wantMode == inputReplaceSearch {
				if m.replaceSearchInput.Value() != "" || m.replaceSearchTerm != "" {
					t.Errorf("stale search state not cleared: %q / %q",
						m.replaceSearchInput.Value(), m.replaceSearchTerm)
				}
			}
		})
	}
}

func TestOpenVaultSearchWithoutEmbedder(t *testing.T) {
	m := model{vaultSearchInput: textinput.New()}
	m.openVaultSearch()
	if m.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", m.inputMode)
	}
	if m.errMsg == "" {
		t.Error("expected a configuration error message")
	}
}

func TestToggleChat(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.toggleChat()
	if !m.chatOpen {
		t.Fatal("chat did not open")
	}
	if m.chatMode != chatModeInput {
		t.Errorf("chatMode = %v, want chatModeInput", m.chatMode)
	}
	m.toggleChat()
	if m.chatOpen {
		t.Error("chat did not close")
	}
}

func TestRejectDiffClearsRunState(t *testing.T) {
	m := model{
		inputMode:         inputPluginDiff,
		pluginDiffOriginal: "before",
		pluginDiffResult:   "after",
		aiRunOnSelection:   true,
	}
	m.rejectDiff()
	if m.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", m.inputMode)
	}
	if m.pluginDiffOriginal != "" || m.pluginDiffResult != "" || m.aiRunOnSelection {
		t.Error("run state not cleared")
	}
}

func TestAcceptDiffReplacesBuffer(t *testing.T) {
	m := model{
		inputMode:        inputPluginDiff,
		editor:           newSelectableEditor(),
		pluginDiffResult: "new content",
	}
	m.editor.SetValue("old content")
	m.acceptDiff()
	if got := m.editor.Value(); got != "new content" {
		t.Errorf("editor value = %q, want %q", got, "new content")
	}
	if m.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", m.inputMode)
	}
	if m.editorMode != modeEdit {
		t.Errorf("editorMode = %v, want modeEdit", m.editorMode)
	}
}

func TestCopyReviewSetsStatus(t *testing.T) {
	m := model{inputMode: inputPluginReview, pluginDiffResult: "the review"}
	m.copyReview()
	if m.errMsg != "Review copied" {
		t.Errorf("errMsg = %q, want %q", m.errMsg, "Review copied")
	}
	if m.inputMode != inputPluginReview {
		t.Error("copy must not leave review mode")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestStartFindReplace|TestOpenVaultSearch|TestToggleChat|TestRejectDiff|TestAcceptDiff|TestCopyReview' -v`
Expected: compile failure — `m.startFindReplace undefined`, etc.

- [ ] **Step 3: Write `actions.go`**

Each body is moved verbatim from its current key handler.

```go
package main

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// The methods in this file are the single implementation of each user-facing
// action. Both the keyboard handlers in model.go and the left-pane button bar
// call them, so the two entry points cannot drift apart.

// startFindReplace opens the find & replace prompt (Ctrl+R). No-op without an
// open note.
func (m *model) startFindReplace() tea.Cmd {
	if !m.noteOpen() {
		return nil
	}
	m.inputMode = inputReplaceSearch
	m.replaceSearchInput.SetValue("")
	m.replaceSearchTerm = ""
	return m.replaceSearchInput.Focus()
}

// openShortcutPicker opens the AI shortcut / fabric pattern picker (Ctrl+G).
// Shortcuts and patterns are re-scanned on every open so additions show up
// without restarting clipad.
func (m *model) openShortcutPicker() tea.Cmd {
	if !m.noteOpen() {
		return nil
	}
	m.shortcuts, _ = loadShortcuts()
	m.fabricPatterns = listFabricPatterns(fabricPatternsDir())
	m.shortcutFilterInput.SetValue("")
	m.inputMode = inputShortcutSelect
	m.shortcutCursor = clampSelectableRow(m.shortcutRows(), 0)
	m.shortcutOffset = 0
	return nil
}

// openVaultSearch opens the semantic vault search modal (Ctrl+T).
func (m *model) openVaultSearch() tea.Cmd {
	if m.indexer == nil || m.indexer.embedder == nil {
		m.errMsg = "Configure embedding_provider in config.toml"
		return nil
	}
	m.inputMode = inputVaultSearch
	m.vaultSearchInput.SetValue("")
	m.vaultSearchResults = nil
	m.vaultSearchCursor = 0
	m.vaultSearchOffset = 0
	return m.vaultSearchInput.Focus()
}

// toggleChat opens or closes the vault chat panel (Ctrl+K), cancelling any
// in-flight agent run on close.
func (m *model) toggleChat() tea.Cmd {
	if m.chatOpen {
		if m.agentCancel != nil {
			m.agentCancel()
			m.agentCancel = nil
		}
		m.chatStreaming = false
		m.agentEvents = nil
		m.chatOpen = false
		m.chatInput.Blur()
		m.recalcLayout()
		return nil
	}
	m.chatOpen = true
	m.chatMode = chatModeInput
	m.recalcLayout()
	return m.chatInput.Focus()
}

// acceptDiff applies the AI result to the buffer and leaves diff mode ("y").
func (m *model) acceptDiff() tea.Cmd {
	if m.aiRunOnSelection {
		// ReplaceSelection records its own op entry.
		m.editor.ReplaceSelection(m.pluginDiffResult)
		m.aiRunOnSelection = false
	} else {
		pre := m.editor.recordOp()
		m.editor.SetValue(m.pluginDiffResult)
		m.editor.commitOp(pre)
	}
	m.editor.ClearSelection()
	// cleanContent unchanged — editor now differs from it, so isDirty() returns true
	m.inputMode = inputNone
	m.pluginActive = nil
	m.pluginDiffOriginal = ""
	m.pluginDiffResult = ""
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	m.syncBarLayout()
	return m.editor.Focus()
}

// rejectDiff discards the AI result and leaves diff mode ("n" / Esc).
func (m *model) rejectDiff() tea.Cmd {
	m.aiRunOnSelection = false
	m.inputMode = inputNone
	m.pluginActive = nil
	m.pluginDiffOriginal = ""
	m.pluginDiffResult = ""
	m.syncBarLayout()
	return nil
}

// copyReview copies the generated review to the system clipboard ("c"). Review
// mode stays open — copying is not a dismissal.
func (m *model) copyReview() tea.Cmd {
	_ = clipboard.WriteAll(m.pluginDiffResult)
	m.errMsg = "Review copied"
	return nil
}
```

- [ ] **Step 4: Add a temporary `syncBarLayout` stub**

`acceptDiff` and `rejectDiff` call it; Task 4 gives it a real body. Add to `buttons.go`:

```go
// syncBarLayout recomputes the bar height and the tree height it takes rows
// from. Filled in by the layout task.
func (m *model) syncBarLayout() {}
```

- [ ] **Step 5: Replace the key handler bodies with calls**

In `model.go`, replace the four cases:

```go
		case "ctrl+r":
			return m, m.startFindReplace()
```

```go
		case "ctrl+g":
			return m, m.openShortcutPicker()
```

```go
		case "ctrl+t":
			return m, m.openVaultSearch()
```

```go
		case "ctrl+k":
			return m, m.toggleChat()
```

In `plugin_diff.go`, replace the `y` and `n` cases:

```go
	case "y":
		return m, m.acceptDiff()
	case "n", "esc":
		return m, m.rejectDiff()
```

In `plugin_review.go`, replace the `c` case:

```go
	case "c":
		return m, m.copyReview()
```

`handlePluginDiff` and `handlePluginReview` have value receivers (`m model`), so calling a pointer method on the local copy is fine and the modified copy is what gets returned.

Remove the now-unused `"github.com/atotto/clipboard"` import from `plugin_review.go`.

- [ ] **Step 6: Add `syncBarLayout` to `closePluginRun`**

In `model.go`, at the end of `closePluginRun`:

```go
func (m *model) closePluginRun(msg string) {
	m.errMsg = msg
	m.inputMode = inputNone
	m.pluginActive = nil
	m.pluginDiffOriginal = ""
	m.pluginDiffResult = ""
	m.syncBarLayout()
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./... -run 'TestStartFindReplace|TestOpenVaultSearch|TestToggleChat|TestRejectDiff|TestAcceptDiff|TestCopyReview' -v`
Expected: PASS

- [ ] **Step 8: Verify the refactor changed no existing behavior**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS, including the existing `plugin_diff_test.go` and `plugin_review_test.go` suites which exercise the `y` / `n` / `c` keys.

---

### Task 3: Citation cursor

**Files:**
- Create: `citations_test.go`
- Modify: `actions.go` (add `chatFocusInput`, `jumpCitation`)
- Modify: `chat.go` (add `nextCiteIndex`)
- Modify: `model.go:1306-1343` (`chatModeView` handler), `model.go:1243-1301` (`enter` in `chatModeInput`), `model.go:1365-1369` (`slashClear`)

**Interfaces:**
- Consumes: `chatCitations(turns []chatTurn) []citation` from Task 1, `model.chatCiteCursor` from Task 1.
- Produces:
  - `func nextCiteIndex(cursor, delta, n int) int`
  - `func (m *model) chatFocusInput() tea.Cmd`
  - `func (m *model) jumpCitation(delta int) tea.Cmd`

- [ ] **Step 1: Write the failing tests**

Create `citations_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextCiteIndex(t *testing.T) {
	tests := []struct {
		name                string
		cursor, delta, n    int
		want                int
	}{
		{"no citations", 0, 1, 0, 0},
		{"first next from none", 0, 1, 3, 1},
		{"first prev from none", 0, -1, 3, 3},
		{"next", 1, 1, 3, 2},
		{"prev", 2, -1, 3, 1},
		{"next wraps", 3, 1, 3, 1},
		{"prev wraps", 1, -1, 3, 3},
		{"single citation next", 1, 1, 1, 1},
		{"single citation prev", 1, -1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextCiteIndex(tt.cursor, tt.delta, tt.n); got != tt.want {
				t.Errorf("nextCiteIndex(%d,%d,%d) = %d, want %d",
					tt.cursor, tt.delta, tt.n, got, tt.want)
			}
		})
	}
}

func TestChatCitations(t *testing.T) {
	turns := []chatTurn{
		{Role: "assistant", Citations: []citation{{Path: "old.md"}}},
		{Role: "user", Content: "again"},
		{Role: "assistant", Citations: []citation{{Path: "new1.md"}, {Path: "new2.md"}}},
	}
	got := chatCitations(turns)
	if len(got) != 2 || got[0].Path != "new1.md" {
		t.Errorf("chatCitations returned %v, want the newest turn's two citations", got)
	}
	if chatCitations(nil) != nil {
		t.Error("chatCitations(nil) should be nil")
	}
	if chatCitations([]chatTurn{{Role: "assistant"}}) != nil {
		t.Error("a turn with no citations should yield nil")
	}
}

// jumpCitationModel builds a model with a real vault file so jumpCitation can
// open it.
func jumpCitationModel(t *testing.T, cites []citation) model {
	t.Helper()
	dir := t.TempDir()
	for _, c := range cites {
		p := filepath.Join(dir, c.Path)
		if err := os.WriteFile(p, []byte("line one\nline two\nline three\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return model{
		vault:     dir,
		editor:    newSelectableEditor(),
		chatOpen:  true,
		chatTurns: []chatTurn{{Role: "assistant", Citations: cites}},
	}
}

func TestJumpCitationOpensFiles(t *testing.T) {
	cites := []citation{
		{Path: "a.md", StartLine: 2, EndLine: 2},
		{Path: "b.md", StartLine: 3, EndLine: 3},
	}

	t.Run("next from none opens the first", func(t *testing.T) {
		m := jumpCitationModel(t, cites)
		m.jumpCitation(1)
		if m.chatCiteCursor != 1 {
			t.Errorf("cursor = %d, want 1", m.chatCiteCursor)
		}
		if filepath.Base(m.currentFile) != "a.md" {
			t.Errorf("currentFile = %q, want a.md", m.currentFile)
		}
		if line, _ := editorCursorPos(m.editor); line != 1 {
			t.Errorf("cursor line = %d, want 1 (0-based for StartLine 2)", line)
		}
	})

	t.Run("prev from none opens the last", func(t *testing.T) {
		m := jumpCitationModel(t, cites)
		m.jumpCitation(-1)
		if m.chatCiteCursor != 2 {
			t.Errorf("cursor = %d, want 2", m.chatCiteCursor)
		}
		if filepath.Base(m.currentFile) != "b.md" {
			t.Errorf("currentFile = %q, want b.md", m.currentFile)
		}
	})

	t.Run("no citations is a no-op", func(t *testing.T) {
		m := model{editor: newSelectableEditor(), chatOpen: true}
		m.jumpCitation(1)
		if m.chatCiteCursor != 0 || m.currentFile != "" {
			t.Error("expected no state change")
		}
	})

	t.Run("dirty buffer detours through the unsaved guard", func(t *testing.T) {
		m := jumpCitationModel(t, cites)
		m.editor.SetValue("unsaved edits")
		m.cleanContent = ""
		m.jumpCitation(1)
		if m.inputMode != inputUnsavedGuard {
			t.Fatalf("inputMode = %v, want inputUnsavedGuard", m.inputMode)
		}
		if m.pendingAction != pendingSwitchFile {
			t.Errorf("pendingAction = %v, want pendingSwitchFile", m.pendingAction)
		}
		if filepath.Base(m.pendingSwitchPath) != "a.md" {
			t.Errorf("pendingSwitchPath = %q, want a.md", m.pendingSwitchPath)
		}
	})
}

func TestChatFocusInput(t *testing.T) {
	m := model{chatOpen: true, chatMode: chatModeView, buttonFocused: true}
	m.chatFocusInput()
	if m.chatMode != chatModeInput {
		t.Errorf("chatMode = %v, want chatModeInput", m.chatMode)
	}
	if m.buttonFocused {
		t.Error("bar focus should move to the chat input")
	}

	closed := model{chatOpen: false, chatMode: chatModeView}
	closed.chatFocusInput()
	if closed.chatMode != chatModeView {
		t.Error("chatFocusInput must be a no-op when the panel is closed")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestNextCiteIndex|TestChatCitations|TestJumpCitation|TestChatFocusInput' -v`
Expected: compile failure — `undefined: nextCiteIndex`, `m.jumpCitation undefined`.

- [ ] **Step 3: Add `nextCiteIndex` to `chat.go`**

```go
// nextCiteIndex advances a 1-based citation cursor by delta, wrapping at both
// ends. A cursor of 0 means "nothing selected yet": stepping forward lands on
// the first citation, stepping back on the last.
func nextCiteIndex(cursor, delta, n int) int {
	if n <= 0 {
		return 0
	}
	if cursor == 0 {
		if delta > 0 {
			return 1
		}
		return n
	}
	i := (cursor - 1 + delta) % n
	if i < 0 {
		i += n
	}
	return i + 1
}
```

- [ ] **Step 4: Add the two actions to `actions.go`**

Add `"path/filepath"` to the imports, then:

```go
// chatFocusInput returns the vault chat panel to input mode ("i" / "/").
func (m *model) chatFocusInput() tea.Cmd {
	if !m.chatOpen {
		return nil
	}
	m.chatMode = chatModeInput
	m.buttonFocused = false
	return m.chatInput.Focus()
}

// jumpCitation steps the citation cursor by delta and opens the citation it
// lands on, exactly as the numbered 1–9 shortcuts do — including the
// unsaved-changes detour, which defers the open until the guard is answered.
func (m *model) jumpCitation(delta int) tea.Cmd {
	cites := chatCitations(m.chatTurns)
	if len(cites) == 0 {
		return nil
	}
	m.chatCiteCursor = nextCiteIndex(m.chatCiteCursor, delta, len(cites))
	c := cites[m.chatCiteCursor-1]
	abs := filepath.Join(m.vault, c.Path)
	if m.isDirty() {
		m.inputMode = inputUnsavedGuard
		m.pendingAction = pendingSwitchFile
		m.pendingSwitchPath = abs
		return nil
	}
	m.buttonFocused = false
	m.openFile(abs)
	m.editor.MoveTo(c.StartLine-1, 0)
	m.activePanel = editorPanel
	m.editorMode = modeEdit
	return m.editor.Focus()
}
```

- [ ] **Step 5: Keep the cursor in sync with the existing chat keys**

In `model.go`'s `handleChatPanel`, `chatModeView` branch, replace the `"i", "/"` case body and the numbered-citation block:

```go
		case "i", "/":
			return m, m.chatFocusInput()
```

```go
		s := msg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			n := int(s[0] - '0')
			cite := mostRecentCitation(m.chatTurns, n)
			if cite != nil {
				m.chatCiteCursor = n
				abs := filepath.Join(m.vault, cite.Path)
				if m.isDirty() {
					m.inputMode = inputUnsavedGuard
					m.pendingAction = pendingSwitchFile
					m.pendingSwitchPath = abs
					return m, nil
				}
				m.openFile(abs)
				m.editor.MoveTo(cite.StartLine-1, 0)
				m.activePanel = editorPanel
				m.editorMode = modeEdit
				return m, m.editor.Focus()
			}
		}
```

The only change is the added `m.chatCiteCursor = n` line, so typing `3` then clicking `Next cite` goes to citation 4.

- [ ] **Step 6: Reset the cursor when the citation list changes**

In `handleChatPanel`, `chatModeInput` branch, immediately after the two turns are appended:

```go
			// Display turns.
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "user", Content: input})
			m.chatTurns = append(m.chatTurns, chatTurn{Role: "assistant"})
			m.chatCiteCursor = 0
```

And in `handleAgentSlash`, `slashClear`:

```go
	case slashClear:
		m.chatTurns = nil
		m.agentMessages = nil
		m.chatCiteCursor = 0
		m.chatViewport.SetContent("")
		return m, nil
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./... -run 'TestNextCiteIndex|TestChatCitations|TestJumpCitation|TestChatFocusInput' -v`
Expected: PASS

- [ ] **Step 8: Verify the whole suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS

---

### Task 4: Rendering and layout

**Files:**
- Modify: `buttons.go` (real `syncBarLayout`, `clampButtonCursor`, `buttonBarView`, styles)
- Modify: `buttons_test.go` (add rendering and layout tests)
- Modify: `model.go:2032-2140` (`recalcLayout`), `model.go:2142-2153` and `model.go:2232-2240` (`View`), `model.go:2388-2419` (`filterView`)
- Modify: `ai_run.go:44-64` (`startAIRun`)
- Modify: `plugin_input.go:123` (diff entry)

**Interfaces:**
- Consumes: `barButtons()`, `barInert()`, `buttonBarHeight()` from Task 1.
- Produces:
  - `func (m *model) syncBarLayout()`
  - `func (m *model) clampButtonCursor()`
  - `func buttonBarView(rows []barButton, height, width int, inert, focused bool, cursor int) string`

- [ ] **Step 1: Write the failing tests**

Append to `buttons_test.go`:

```go
func TestSyncBarLayoutShrinksTree(t *testing.T) {
	tests := []struct {
		name           string
		collapsed      bool
		treeHidden     bool
		wantBar        int
		wantTreeHeight int
	}{
		{"expanded takes five rows", false, false, 5, 40 - 2 - 5},
		{"collapsed takes one row", true, false, 1, 40 - 2 - 1},
		{"tree hidden takes none", false, true, 0, 40 - 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBarTestModel(t, 120, 40)
			m.buttonsCollapsed = tt.collapsed
			m.treeHidden = tt.treeHidden
			m.recalcLayout()
			if m.barHeight != tt.wantBar {
				t.Errorf("barHeight = %d, want %d", m.barHeight, tt.wantBar)
			}
			if m.treeHeight != tt.wantTreeHeight {
				t.Errorf("treeHeight = %d, want %d", m.treeHeight, tt.wantTreeHeight)
			}
			if m.tree.height != m.treeHeight {
				t.Errorf("tree.height = %d, want %d", m.tree.height, m.treeHeight)
			}
			if m.barHeight > 0 && m.treeHeight+m.barHeight != m.height-2 {
				t.Errorf("left column = %d rows, want %d", m.treeHeight+m.barHeight, m.height-2)
			}
		})
	}
}

func TestSyncBarLayoutDropsFocusWhenBarDisappears(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.buttonFocused = true
	m.buttonCursor = 3
	m.treeHidden = true
	m.recalcLayout()
	if m.buttonFocused {
		t.Error("focus should drop when the bar has no rows")
	}
}

func TestClampButtonCursor(t *testing.T) {
	tests := []struct {
		name      string
		barHeight int
		cursor    int
		want      int
	}{
		{"in range", 5, 3, 3},
		{"past the end", 5, 9, 4},
		{"negative", 5, -2, 0},
		{"no bar", 0, 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{barHeight: tt.barHeight, buttonCursor: tt.cursor}
			m.clampButtonCursor()
			if m.buttonCursor != tt.want {
				t.Errorf("buttonCursor = %d, want %d", m.buttonCursor, tt.want)
			}
		})
	}
}

func TestButtonBarViewRowCount(t *testing.T) {
	rows := []barButton{
		{label: "Find & replace", action: actionFindReplace, enabled: true},
		{label: "AI tools", action: actionAITools, enabled: true},
	}
	tests := []struct {
		name    string
		height  int
		wantLen int
	}{
		{"expanded", 3, 3},
		{"collapsed", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buttonBarView(rows, tt.height, 20, false, false, 0)
			if got := len(strings.Split(out, "\n")); got != tt.wantLen {
				t.Errorf("rendered %d lines, want %d:\n%s", got, tt.wantLen, out)
			}
		})
	}
}

func TestButtonBarViewToggleGlyph(t *testing.T) {
	rows := []barButton{{label: "Ask", action: actionAsk, enabled: true}}
	expanded := buttonBarView(rows, 2, 20, false, false, 0)
	if !strings.Contains(expanded, "[-]") {
		t.Errorf("expanded bar should show [-]:\n%s", expanded)
	}
	collapsed := buttonBarView(rows, 1, 20, false, false, 0)
	if !strings.Contains(collapsed, "[+]") {
		t.Errorf("collapsed bar should show [+]:\n%s", collapsed)
	}
}

func TestButtonBarViewShowsLabels(t *testing.T) {
	rows := []barButton{
		{label: "Find & replace", action: actionFindReplace, enabled: true},
		{label: "AI tools", action: actionAITools, enabled: false},
	}
	out := buttonBarView(rows, 3, 24, false, false, 0)
	for _, want := range []string{"Find & replace", "AI tools"} {
		if !strings.Contains(out, want) {
			t.Errorf("bar is missing %q:\n%s", want, out)
		}
	}
}

func TestButtonBarViewTruncatesLongLabels(t *testing.T) {
	rows := []barButton{{label: "Find & replace", action: actionFindReplace, enabled: true}}
	out := buttonBarView(rows, 2, 10, false, false, 0)
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 10 {
			t.Errorf("line wider than the pane: %q (%d cols)", line, lipgloss.Width(line))
		}
	}
}
```

Add `"strings"` and `"github.com/charmbracelet/lipgloss"` to `buttons_test.go`'s imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestSyncBarLayout|TestClampButtonCursor|TestButtonBarView' -v`
Expected: compile failure — `undefined: buttonBarView`, and `syncBarLayout` does nothing.

- [ ] **Step 3: Implement the layout helpers in `buttons.go`**

Replace the stub:

```go
// clampButtonCursor keeps the focus cursor inside the bar's current rows.
func (m *model) clampButtonCursor() {
	if m.barHeight <= 0 {
		m.buttonCursor = 0
		return
	}
	if m.buttonCursor >= m.barHeight {
		m.buttonCursor = m.barHeight - 1
	}
	if m.buttonCursor < 0 {
		m.buttonCursor = 0
	}
}

// syncBarLayout recomputes the bar height and the tree height it takes rows
// from. The active button set depends on the mode, so this runs on every mode
// switch — it is the cheap part of recalcLayout, without the viewport rebuilds
// that would reset diff and chat scroll positions on every transition.
func (m *model) syncBarLayout() {
	avail := m.height - 2
	m.barHeight = buttonBarHeight(m.treeWidth, avail, m.buttonsCollapsed, len(m.barButtons()))
	m.treeHeight = avail - m.barHeight
	if m.treeHeight < 1 {
		m.treeHeight = 1
	}
	m.tree.height = m.treeHeight
	m.tree.clampOffset()
	if m.barHeight == 0 {
		m.buttonFocused = false
	}
	m.clampButtonCursor()
}
```

- [ ] **Step 4: Implement the view in `buttons.go`**

Add the imports `"strings"`, `"github.com/charmbracelet/lipgloss"`, `"github.com/charmbracelet/x/ansi"` and:

```go
var (
	// buttonBarStyle mirrors treePanelStyle so the vertical divider between
	// the left pane and the editor runs unbroken past the bar.
	buttonBarStyle = lipgloss.NewStyle().
			Padding(0, 1).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder())

	buttonRuleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	buttonLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	buttonDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// buttonBarView renders the bar at exactly `height` rows and `width` columns
// (plus its right border), so the left column always joins flush with the
// editor. Row 0 is the toggle rule; rows 1.. are buttons, of which only
// height-1 fit. `inert` dims everything, `focused`+`cursor` highlight one row.
func buttonBarView(rows []barButton, height, width int, inert, focused bool, cursor int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	maxW := width - 2 // buttonBarStyle's horizontal padding
	if maxW < 1 {
		maxW = 1
	}

	fit := func(s string) string {
		if lipgloss.Width(s) > maxW {
			return ansi.Truncate(s, maxW, "…")
		}
		return s
	}
	pad := func(s string) string {
		if w := lipgloss.Width(s); w < maxW {
			return s + strings.Repeat(" ", maxW-w)
		}
		return s
	}

	// The toggle glyph reflects what is on screen: a bar squeezed down to its
	// rule row by a short terminal reads as collapsed, because it is.
	glyph := "[+]"
	if height > 1 {
		glyph = "[-]"
	}
	rule := fit("─" + glyph + strings.Repeat("─", maxW))

	lines := make([]string, 0, height)
	renderRow := func(i int, text string, enabled bool) {
		switch {
		case focused && i == cursor:
			text = treeSelectedStyle.Render(pad(text))
		case inert || !enabled:
			text = buttonDisabledStyle.Render(text)
		case i == 0:
			text = buttonRuleStyle.Render(text)
		default:
			text = buttonLabelStyle.Render(text)
		}
		lines = append(lines, text)
	}
	renderRow(0, rule, true)
	for i := 0; i < height-1 && i < len(rows); i++ {
		renderRow(i+1, fit(" "+rows[i].label), rows[i].enabled)
	}

	return buttonBarStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}
```

- [ ] **Step 5: Wire the height into `recalcLayout`**

In `model.go`, delete the `m.treeHeight` block at the top of `recalcLayout` (lines 2033-2036) — the editor height stays where it is:

```go
func (m *model) recalcLayout() {
	m.editorHeight = m.height - 2
	if m.editorHeight < 1 {
		m.editorHeight = 1
	}
```

Then replace the tree sizing block near the end (currently `m.tree.width`/`m.tree.height`/`m.tree.clampOffset`):

```go
	m.tree.width = m.treeWidth
	// treeWidth is final here, so the bar knows whether there is a left pane.
	m.syncBarLayout()
```

`syncBarLayout` sets `m.tree.height` and calls `clampOffset` itself.

- [ ] **Step 6: Join the bar into the left column in `View`**

In `model.go`'s `View`, replace the tree-view block:

```go
	var treeView string
	if m.treeWidth > 0 {
		treeView = m.tree.View(m.activePanel == treePanel && !m.buttonFocused)
		if m.inputMode == inputFilter {
			treeView = m.filterView()
		}
		if m.barHeight > 0 {
			bar := buttonBarView(m.barButtons(), m.barHeight, m.treeWidth,
				m.barInert(), m.buttonFocused, m.buttonCursor)
			treeView = lipgloss.JoinVertical(lipgloss.Left, treeView, bar)
		}
	}
```

Passing `!m.buttonFocused` keeps the tree from drawing its own cursor highlight while the bar owns focus — two highlights at once read as two cursors.

- [ ] **Step 7: Fix `filterView` to fill its height**

In `model.go`'s `filterView`, change the final line:

```go
	return treePanelStyle.Width(m.treeWidth).Height(m.treeHeight).MaxHeight(m.treeHeight).Render(b.String())
```

Without `Height`, a short result list lets the bar float up mid-column and the left column ends up shorter than the editor.

- [ ] **Step 8: Sync the bar on diff and review entry**

In `ai_run.go`'s `startAIRun`, after the `if run.review { ... } else { ... }` block and before `chunks, errs := run.start(...)`:

```go
	m.syncBarLayout()
```

In `plugin_input.go`, immediately after the line that sets `m.inputMode = inputPluginDiff` (line 123):

```go
		m.syncBarLayout()
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./... -run 'TestSyncBarLayout|TestClampButtonCursor|TestButtonBarView' -v`
Expected: PASS

- [ ] **Step 10: Fix fallout in the existing suite**

Run: `go build ./... && go vet ./... && go test ./...`

Existing tests that call `recalcLayout` and then assert on `treeHeight`, tree scroll offsets, or rendered row counts will now see a shorter tree — that is the intended change. For each failure, update the expectation to account for the bar (`m.height - 2 - m.barHeight`) rather than reverting the behavior. Do **not** change an assertion whose failure indicates a real layout bug (e.g. the left column no longer totalling `m.height - 2`).

Expected: all PASS

---

### Task 5: Mouse routing and activation

**Files:**
- Modify: `buttons.go` (`buttonBarRowAt`, `handleButtonBarMouse`, `activateBarRow`, `runBarAction`)
- Modify: `buttons_test.go`
- Modify: `mouse.go:315-343` (`handleMouseMsg`)
- Modify: `model.go:699-712` (the `tea.MouseMsg` branch)

**Interfaces:**
- Consumes: everything from Tasks 1-4.
- Produces:
  - `func (m model) buttonBarRowAt(x, y int) (int, bool)`
  - `func handleButtonBarMouse(m model, row int, msg tea.MouseMsg) (tea.Model, tea.Cmd)`
  - `func (m model) activateBarRow(row int) (model, tea.Cmd)`
  - `func (m model) runBarAction(a barAction) (model, tea.Cmd)`

- [ ] **Step 1: Write the failing tests**

Append to `buttons_test.go`:

```go
func TestButtonBarRowAt(t *testing.T) {
	// width 120, height 40 → treeWidth 30, status bar at y=39.
	// With four buttons: barHeight 5, treeHeight 33 → bar rows are y=33..37.
	tests := []struct {
		name    string
		mutate  func(m *model)
		x, y    int
		wantRow int
		wantOK  bool
	}{
		{"inside the tree", nil, 5, 10, 0, false},
		{"first bar row", nil, 5, 33, 0, true},
		{"a button row", nil, 5, 35, 2, true},
		{"last bar row", nil, 5, 37, 4, true},
		{"status bar row", nil, 5, 39, 0, false},
		{"editor column", nil, 60, 35, 0, false},
		{"tree border column", nil, 30, 35, 0, false},
		{"tree hidden", func(m *model) { m.treeHidden = true; m.recalcLayout() }, 5, 35, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBarTestModel(t, 120, 40)
			if tt.mutate != nil {
				tt.mutate(&m)
			}
			row, ok := m.buttonBarRowAt(tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && row != tt.wantRow {
				t.Errorf("row = %d, want %d", row, tt.wantRow)
			}
		})
	}
}

func TestActivateBarRowToggle(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	before := m.treeHeight

	m2, _ := m.activateBarRow(0)
	if !m2.buttonsCollapsed {
		t.Fatal("row 0 should collapse the bar")
	}
	if m2.barHeight != 1 {
		t.Errorf("barHeight = %d, want 1", m2.barHeight)
	}
	if m2.treeHeight <= before {
		t.Errorf("collapsing should give rows back to the tree: %d → %d", before, m2.treeHeight)
	}

	m3, _ := m2.activateBarRow(0)
	if m3.buttonsCollapsed {
		t.Error("a second toggle should expand the bar again")
	}
	if m3.treeHeight != before {
		t.Errorf("treeHeight = %d, want %d", m3.treeHeight, before)
	}
}

func TestActivateBarRowRunsActions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(m *model)
		row   int
		check func(t *testing.T, m model)
	}{
		{
			name:  "find & replace",
			setup: func(m *model) { m.currentFile = "/v/a.md" },
			row:   1,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputReplaceSearch {
					t.Errorf("inputMode = %v, want inputReplaceSearch", m.inputMode)
				}
			},
		},
		{
			name:  "disabled button does nothing",
			setup: func(m *model) {},
			row:   1,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputNone {
					t.Errorf("inputMode = %v, want inputNone", m.inputMode)
				}
			},
		},
		{
			name:  "reject in diff mode",
			setup: func(m *model) { m.inputMode = inputPluginDiff; m.pluginDiffResult = "x" },
			row:   2,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputNone || m.pluginDiffResult != "" {
					t.Error("reject should clear the run")
				}
			},
		},
		{
			name:  "row past the end",
			setup: func(m *model) { m.currentFile = "/v/a.md" },
			row:   99,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputNone {
					t.Error("out-of-range row should be a no-op")
				}
			},
		},
		{
			name:  "inert bar ignores activation",
			setup: func(m *model) { m.currentFile = "/v/a.md"; m.inputMode = inputVaultSearch },
			row:   1,
			check: func(t *testing.T, m model) {
				if m.inputMode != inputVaultSearch {
					t.Error("a modal overlay must not be replaced by a button action")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newBarTestModel(t, 120, 40)
			tt.setup(&m)
			m.recalcLayout()
			got, _ := m.activateBarRow(tt.row)
			tt.check(t, got)
		})
	}
}

func TestHandleButtonBarMouseIgnoresNonPress(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	for _, msg := range []tea.MouseMsg{
		{Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
		{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
		{Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
	} {
		got, _ := handleButtonBarMouse(m, 0, msg)
		if got.(model).buttonsCollapsed {
			t.Errorf("%v should not toggle the bar", msg)
		}
	}
}

func TestHandleMouseMsgRoutesToBar(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	msg := tea.MouseMsg{X: 5, Y: m.treeHeight, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	got, _ := handleMouseMsg(m, msg)
	if !got.(model).buttonsCollapsed {
		t.Error("a click on the bar's toggle row should reach the bar, not the tree")
	}
}
```

Add `tea "github.com/charmbracelet/bubbletea"` to `buttons_test.go`'s imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestButtonBarRowAt|TestActivateBarRow|TestHandleButtonBarMouse|TestHandleMouseMsgRoutesToBar' -v`
Expected: compile failure — `m.buttonBarRowAt undefined`, etc.

- [ ] **Step 3: Implement hit-testing and activation in `buttons.go`**

Add `tea "github.com/charmbracelet/bubbletea"` to the imports and:

```go
// buttonBarRowAt maps an absolute terminal coordinate to a bar row. The left
// column is split at m.treeHeight: above is the file tree, below is the bar.
func (m model) buttonBarRowAt(x, y int) (int, bool) {
	if m.barHeight <= 0 {
		return 0, false
	}
	hit, _, localY, ok := hitTestPanel(m.treeWidth, m.chatWidth, m.width, m.height, x, y)
	if !ok || hit != treePanel || localY < m.treeHeight {
		return 0, false
	}
	row := localY - m.treeHeight
	if row >= m.barHeight {
		return 0, false
	}
	return row, true
}

// handleButtonBarMouse routes a mouse event that landed on the bar. Only a
// left-button press activates; wheel events over the bar are ignored rather
// than scrolling the tree behind it.
func handleButtonBarMouse(m model, row int, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	mm, cmd := m.activateBarRow(row)
	return mm, cmd
}

// activateBarRow runs the row's action. Row 0 is the collapse toggle.
func (m model) activateBarRow(row int) (model, tea.Cmd) {
	if m.barInert() {
		return m, nil
	}
	if row == 0 {
		m.buttonsCollapsed = !m.buttonsCollapsed
		m.syncBarLayout()
		return m, nil
	}
	rows := m.barButtons()
	i := row - 1
	if i < 0 || i >= len(rows) || !rows[i].enabled {
		return m, nil
	}
	return m.runBarAction(rows[i].action)
}

// runBarAction dispatches to the shared action methods in actions.go, so a
// button and its keyboard shortcut run exactly the same code.
func (m model) runBarAction(a barAction) (model, tea.Cmd) {
	var cmd tea.Cmd
	switch a {
	case actionFindReplace:
		cmd = m.startFindReplace()
	case actionAITools:
		cmd = m.openShortcutPicker()
	case actionAsk:
		cmd = m.toggleChat()
	case actionAISearch:
		cmd = m.openVaultSearch()
	case actionApprove:
		cmd = m.acceptDiff()
	case actionReject:
		cmd = m.rejectDiff()
	case actionCopyReview:
		cmd = m.copyReview()
	case actionChatInput:
		cmd = m.chatFocusInput()
	case actionPrevCite:
		cmd = m.jumpCitation(-1)
	case actionNextCite:
		cmd = m.jumpCitation(1)
	}
	// Opening a modal makes the bar inert, and several actions move focus
	// elsewhere; either way the bar must not keep keyboard focus.
	if m.barInert() || m.barHeight == 0 {
		m.buttonFocused = false
	}
	m.syncBarLayout()
	return m, cmd
}
```

- [ ] **Step 4: Route bar clicks in `mouse.go`**

In `handleMouseMsg`, change the `treePanel` case:

```go
	case treePanel:
		if localY >= m.treeHeight {
			return handleButtonBarMouse(m, localY-m.treeHeight, msg)
		}
		return handleTreeMouse(m, localY, msg)
```

- [ ] **Step 5: Carve the bar out of the diff/review mouse path**

In `model.go`'s `tea.MouseMsg` branch, `handlePaneMouse` currently claims every event in those modes. Give the bar first refusal:

```go
		if m.inputMode == inputPluginReview || m.inputMode == inputPluginDiff {
			if row, ok := m.buttonBarRowAt(msg.X, msg.Y); ok {
				return handleButtonBarMouse(m, row, msg)
			}
			return m.handlePaneMouse(msg)
		}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./... -run 'TestButtonBarRowAt|TestActivateBarRow|TestHandleButtonBarMouse|TestHandleMouseMsgRoutesToBar' -v`
Expected: PASS

- [ ] **Step 7: Verify the whole suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS. `mouse_test.go`'s existing tree-click tests call `handleTreeMouse` directly and are unaffected; any test going through `handleMouseMsg` at a large Y may now land on the bar — update those coordinates to stay inside `m.treeHeight`.

---

### Task 6: Keyboard focus and the Tab cycle

**Files:**
- Modify: `buttons.go` (`handleButtonKeys`)
- Modify: `buttons_test.go`
- Modify: `model.go:714-742` (KeyMsg routing), `model.go:984-995` (`tab` case)
- Modify: `plugin_diff.go` (`tab` case), `plugin_review.go` (`tab` case)

**Interfaces:**
- Consumes: `activateBarRow`, `clampButtonCursor`, `barHeight` from Tasks 1-5.
- Produces: `func (m model) handleButtonKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool)`

- [ ] **Step 1: Write the failing tests**

Append to `buttons_test.go`:

```go
func barFocusModel(t *testing.T) model {
	t.Helper()
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md" // enables Find & replace and AI tools
	m.buttonFocused = true
	return m
}

func TestHandleButtonKeysNotFocused(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	if _, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyDown}); handled {
		t.Error("keys must fall through when the bar is not focused")
	}
}

func TestHandleButtonKeysCursor(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		keys       []tea.KeyMsg
		wantCursor int
	}{
		{"down moves", 0, []tea.KeyMsg{{Type: tea.KeyDown}}, 1},
		{"up moves", 2, []tea.KeyMsg{{Type: tea.KeyUp}}, 1},
		{"up stops at the toggle row", 0, []tea.KeyMsg{{Type: tea.KeyUp}}, 0},
		{"down stops at the last row", 4, []tea.KeyMsg{{Type: tea.KeyDown}}, 4},
		{"walks the whole bar", 0, []tea.KeyMsg{
			{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyDown},
		}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := barFocusModel(t)
			m.buttonCursor = tt.start
			for _, k := range tt.keys {
				mm, _, handled := m.handleButtonKeys(k)
				if !handled {
					t.Fatalf("key %v not handled", k)
				}
				m = mm.(model)
			}
			if m.buttonCursor != tt.wantCursor {
				t.Errorf("buttonCursor = %d, want %d", m.buttonCursor, tt.wantCursor)
			}
		})
	}
}

func TestHandleButtonKeysActivate(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeySpace}} {
		m := barFocusModel(t)
		m.buttonCursor = 1 // Find & replace
		mm, _, handled := m.handleButtonKeys(key)
		if !handled {
			t.Fatalf("%v not handled", key)
		}
		got := mm.(model)
		if got.inputMode != inputReplaceSearch {
			t.Errorf("%v: inputMode = %v, want inputReplaceSearch", key, got.inputMode)
		}
		if got.buttonFocused {
			t.Errorf("%v: focus should leave the bar when a modal opens", key)
		}
	}
}

func TestHandleButtonKeysEscReturnsToTree(t *testing.T) {
	m := barFocusModel(t)
	m.buttonCursor = 2
	mm, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatal("esc not handled")
	}
	got := mm.(model)
	if got.buttonFocused {
		t.Error("esc should drop bar focus")
	}
	if got.activePanel != treePanel {
		t.Errorf("activePanel = %v, want treePanel", got.activePanel)
	}
}

func TestHandleButtonKeysSwallowsOtherKeys(t *testing.T) {
	m := barFocusModel(t)
	m.editor.SetValue("")
	mm, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !handled {
		t.Fatal("stray keys must be swallowed, not passed to the editor")
	}
	if got := mm.(model).editor.Value(); got != "" {
		t.Errorf("editor value = %q, want empty", got)
	}
}

func TestHandleButtonKeysCtrlQFallsThrough(t *testing.T) {
	m := barFocusModel(t)
	if _, _, handled := m.handleButtonKeys(tea.KeyMsg{Type: tea.KeyCtrlQ}); handled {
		t.Error("ctrl+q must reach the global quit guard")
	}
}

func TestTabCycleNormalMode(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md"
	m.activePanel = treePanel

	// tree → bar
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if !m.buttonFocused {
		t.Fatal("tab from the tree should focus the bar")
	}
	if m.buttonCursor != 0 {
		t.Errorf("buttonCursor = %d, want 0", m.buttonCursor)
	}

	// bar → editor
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.buttonFocused || m.activePanel != editorPanel {
		t.Fatalf("tab from the bar should focus the editor: focused=%v panel=%v",
			m.buttonFocused, m.activePanel)
	}

	// editor → tree
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.activePanel != treePanel || m.buttonFocused {
		t.Fatalf("tab from the editor should return to the tree: panel=%v", m.activePanel)
	}
}

func TestTabCycleSkipsHiddenBar(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md"
	m.treeHidden = true
	m.recalcLayout()
	m.activePanel = treePanel
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := mm.(model)
	if got.buttonFocused {
		t.Error("there is no bar to focus when the tree is hidden")
	}
	if got.activePanel != editorPanel {
		t.Errorf("activePanel = %v, want editorPanel", got.activePanel)
	}
}

func TestTabCycleDiffMode(t *testing.T) {
	m := newBarTestModel(t, 120, 40)
	m.currentFile = "/v/a.md"
	m.inputMode = inputPluginDiff
	m.paneFocus = paneFocusRight
	m.recalcLayout()

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.paneFocus != paneFocusLeft || m.buttonFocused {
		t.Fatalf("right → left expected, got focus=%v bar=%v", m.paneFocus, m.buttonFocused)
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if !m.buttonFocused {
		t.Fatal("left → bar expected")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mm.(model)
	if m.buttonFocused || m.paneFocus != paneFocusRight {
		t.Fatalf("bar → right expected, got focus=%v bar=%v", m.paneFocus, m.buttonFocused)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run 'TestHandleButtonKeys|TestTabCycle' -v`
Expected: compile failure — `m.handleButtonKeys undefined`.

- [ ] **Step 3: Implement `handleButtonKeys` in `buttons.go`**

```go
// handleButtonKeys handles a keystroke while the bar has keyboard focus. The
// bool reports whether the key was consumed; false means the caller should run
// its normal routing. Every unrecognised key is consumed on purpose — without
// that, a keystroke aimed at the bar would fall through to the editor and type
// into the buffer.
func (m model) handleButtonKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if !m.buttonFocused {
		return m, nil, false
	}
	switch msg.String() {
	case "ctrl+q":
		return m, nil, false // the global quit guard owns this
	case "up":
		if m.buttonCursor > 0 {
			m.buttonCursor--
		}
		return m, nil, true
	case "down":
		if m.buttonCursor < m.barHeight-1 {
			m.buttonCursor++
		}
		return m, nil, true
	case "enter", " ":
		mm, cmd := m.activateBarRow(m.buttonCursor)
		return mm, cmd, true
	case "esc":
		m.buttonFocused = false
		m.activePanel = treePanel
		m.editor.Blur()
		return m, nil, true
	case "tab":
		m.buttonFocused = false
		switch m.inputMode {
		case inputPluginDiff, inputPluginReview:
			m.paneFocus = paneFocusRight
			return m, nil, true
		}
		m.activePanel = editorPanel
		if m.editorMode == modeEdit && m.noteOpen() {
			return m, m.editor.Focus(), true
		}
		return m, nil, true
	}
	return m, nil, true
}
```

- [ ] **Step 4: Route bar keys in `Update`**

In `model.go`'s `tea.KeyMsg` branch, immediately after the `m.pluginProcessing` block and **before** `if m.inputMode != inputNone`:

```go
		if mm, cmd, handled := m.handleButtonKeys(msg); handled {
			return mm, cmd
		}
```

Placing it before the input-mode dispatch is what lets the bar keep focus inside diff and review mode.

- [ ] **Step 5: Add the bar to the normal Tab cycle**

Replace the `case "tab":` block in `model.go`:

```go
		case "tab":
			if m.activePanel == treePanel {
				if m.barHeight > 0 {
					m.buttonFocused = true
					m.buttonCursor = 0
					m.editor.Blur()
					return m, nil
				}
				m.activePanel = editorPanel
				if m.editorMode == modeEdit {
					return m, m.editor.Focus()
				}
				return m, nil
			}
			m.activePanel = treePanel
			m.editor.Blur()
			return m, nil
```

- [ ] **Step 6: Add the bar to the diff and review Tab cycles**

In `plugin_diff.go`'s `handlePluginDiff` and `plugin_review.go`'s `handlePluginReview`, replace the `tab` case in both with the identical block:

```go
	case "tab":
		// right pane → left pane → button bar → right pane
		if m.paneFocus == paneFocusLeft && m.barHeight > 0 {
			m.buttonFocused = true
			m.buttonCursor = 0
			return m, nil
		}
		m.paneFocus = togglePaneFocus(m.paneFocus)
		return m, nil
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./... -run 'TestHandleButtonKeys|TestTabCycle' -v`
Expected: PASS

- [ ] **Step 8: Verify the whole suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS

---

### Task 7: Documentation and end-to-end verification

**Files:**
- Modify: `help_modal.go:37-128` (`helpSections`)
- Modify: `README.md`

**Interfaces:**
- Consumes: everything.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Add a help section**

In `help_modal.go`, insert a new section into `helpSections` immediately after the `"File Tree"` section:

```go
	{
		title: "Button Bar (left pane)",
		entries: []helpEntry{
			{"Click", "Run the button's action"},
			{"Click [-] / [+]", "Collapse or expand the bar"},
			{"Tab", "Focus the bar (tree → bar → editor)"},
			{"Up / Down", "Move between buttons"},
			{"Enter / Space", "Activate the focused button"},
			{"Esc", "Return focus to the file tree"},
		},
	},
```

- [ ] **Step 2: Verify the help modal still renders**

Run: `go test ./... -run TestHelp -v`
Expected: PASS. If `help_modal_test.go` asserts a section count or an exact rendered height, update it for the new section.

- [ ] **Step 3: Document the bar in the README**

Add a section after the file-tree documentation:

```markdown
### Button bar

The bottom of the left pane carries a bar of clickable buttons. In ordinary
use it offers **Find & replace**, **AI tools**, **Ask**, and **AI search** —
the same actions as `Ctrl+R`, `Ctrl+G`, `Ctrl+K`, and `Ctrl+T`. It changes with
the mode: **Approve** / **Reject** in the diff view, **Copy** in the review
view, and **Input** / **Prev cite** / **Next cite** while the vault chat panel
is open.

Click the `[-]` on the bar's top rule to minify it to a single row and give the
space back to the file tree; `[+]` restores it. The file tree scrolls inside
whatever room is left, so the bar is always on screen.

The bar joins the `Tab` focus cycle (file tree → bar → editor). While focused,
`↑`/`↓` move between buttons, `Enter` or `Space` activates one, and `Esc`
returns focus to the tree.
```

- [ ] **Step 4: Full verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS

Run: `gofmt -l .`
Expected: no output

- [ ] **Step 5: Manual smoke test**

Build and run against a scratch vault:

```bash
go build -o /tmp/clipad-p11 . && /tmp/clipad-p11
```

Confirm by hand:
1. The bar sits below the tree with four buttons; the tree scrolls above it.
2. Clicking each button does what its keyboard shortcut does.
3. `[-]` collapses to one row and the tree grows; `[+]` restores.
4. `Ctrl+B` hides the tree and the bar with it.
5. Running an AI shortcut that produces a diff swaps the bar to Approve/Reject, and clicking them works.
6. A review-type shortcut shows Copy, and clicking it copies.
7. `Ctrl+K` swaps the bar to Input / Prev cite / Next cite; after an answer with citations, the cite buttons step through them.
8. `Tab` walks tree → bar → editor and back.
9. Resizing the terminal very short does not corrupt the layout.

---

## Notes for the executor

- **Do not run any git command.** The user commits and releases manually, and
  the working tree contains unrelated uncommitted changes.
- `model.go` is 2561 lines. Everything new belongs in `buttons.go` or
  `actions.go`; only routing and state changes go into `model.go`.
- When an existing test fails after a layout change, work out whether the new
  value is correct before editing the assertion. `treeHeight` legitimately
  shrinks by `barHeight`; the left column totalling anything other than
  `m.height - 2` is a bug.
