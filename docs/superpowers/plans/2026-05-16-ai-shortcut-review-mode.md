# AI Shortcut Review Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give each AI shortcut a type — `replace` (existing diff+accept flow) or `review` (read-only side-by-side view that never modifies the note).

**Architecture:** Add a `Type` field to `AIShortcut` with read-time resolution (no config migration). Reuse the existing streaming machinery and two-viewport layout; route to a new `inputPluginReview` mode for review shortcuts. Add a `type` step to the shortcut create/edit chain. Built-in defaults `critique`, `questions`, `risks`, `outline` ship as `review`; the other 19 as `replace`.

**Tech Stack:** Go, Bubble Tea, Lipgloss, `github.com/charmbracelet/bubbles/viewport`, `github.com/atotto/clipboard`, `github.com/pelletier/go-toml/v2`. Tests use the standard `testing` package with the existing `newTestModel(t)` / `pressEnter()` / `pressEsc()` helpers in `shortcuts_input_test.go`.

This is the clipad repo at `/home/kc/repos/clipad`. The package is `main` (all `.go` files are flat in the repo root). Run all tests with `go test ./...` and a single test with `go test -run TestName .`.

---

### Task 1: `Type` field, `resolveShortcutType`, and updated defaults

**Files:**
- Modify: `shortcuts.go` (the `AIShortcut` struct at lines 16-20; add helper + name set)
- Modify: `defaults/ai_shortcuts.toml` (add `type` to every entry)
- Test: `shortcuts_test.go` (append tests)

- [ ] **Step 1: Write the failing test for `resolveShortcutType`**

Append to `shortcuts_test.go`:

```go
func TestResolveShortcutType(t *testing.T) {
	cases := []struct {
		name string
		in   AIShortcut
		want string
	}{
		{"explicit replace", AIShortcut{Name: "x", Type: "replace"}, "replace"},
		{"explicit review", AIShortcut{Name: "x", Type: "review"}, "review"},
		{"empty critique infers review", AIShortcut{Name: "critique"}, "review"},
		{"empty questions infers review", AIShortcut{Name: "questions"}, "review"},
		{"empty risks infers review", AIShortcut{Name: "risks"}, "review"},
		{"empty outline infers review", AIShortcut{Name: "outline"}, "review"},
		{"empty replace built-in", AIShortcut{Name: "tighten"}, "replace"},
		{"empty custom name", AIShortcut{Name: "my-custom"}, "replace"},
		{"unrecognised type string", AIShortcut{Name: "critique", Type: "bogus"}, "review"},
		{"unrecognised type custom name", AIShortcut{Name: "foo", Type: "bogus"}, "replace"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveShortcutType(c.in); got != c.want {
				t.Errorf("resolveShortcutType(%+v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestResolveShortcutType .`
Expected: build failure — `undefined: resolveShortcutType` and `unknown field Type in struct literal`.

- [ ] **Step 3: Add the `Type` field and `resolveShortcutType`**

In `shortcuts.go`, change the struct (lines 16-20) to:

```go
type AIShortcut struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`
	Type        string `toml:"type"`
}
```

Then add, immediately after the struct:

```go
// inferredReviewNames is the set of built-in shortcut names that resolve to
// the "review" type when a shortcut has no explicit type (e.g. configs saved
// before the type field existed).
var inferredReviewNames = map[string]bool{
	"critique":  true,
	"questions": true,
	"risks":     true,
	"outline":   true,
}

// resolveShortcutType returns the effective type for a shortcut: "replace"
// or "review". An explicit valid type wins; otherwise the type is inferred
// from the shortcut name.
func resolveShortcutType(s AIShortcut) string {
	switch s.Type {
	case "replace", "review":
		return s.Type
	}
	if inferredReviewNames[s.Name] {
		return "review"
	}
	return "replace"
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestResolveShortcutType .`
Expected: PASS.

- [ ] **Step 5: Write the failing test for typed defaults**

Append to `shortcuts_test.go`:

```go
func TestDefaultShortcuts_HaveResolvedTypes(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}

	wantReview := map[string]bool{"critique": true, "questions": true, "risks": true, "outline": true}
	seenReview := map[string]bool{}
	for _, s := range loaded {
		got := resolveShortcutType(s)
		if wantReview[s.Name] {
			if got != "review" {
				t.Errorf("%q: type = %q, want review", s.Name, got)
			}
			seenReview[s.Name] = true
		} else if got != "replace" {
			t.Errorf("%q: type = %q, want replace", s.Name, got)
		}
	}
	for name := range wantReview {
		if !seenReview[name] {
			t.Errorf("expected built-in %q not found in defaults", name)
		}
	}
}
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test -run TestDefaultShortcuts_HaveResolvedTypes .`
Expected: PASS already (resolution infers correctly even before the TOML has explicit types). This test pins the behaviour and guards the next step. If it passes, continue; the TOML update below makes the types explicit and self-documenting.

- [ ] **Step 7: Add explicit `type` to every entry in `defaults/ai_shortcuts.toml`**

Add a `type = '...'` line to each of the 23 `[[shortcuts]]` blocks. Use `type = 'review'` for exactly these four: `critique`, `questions`, `risks`, `outline`. Use `type = 'replace'` for all others: `prd`, `userstory`, `acceptance`, `todos`, `prioritize`, `breakdown`, `onboard`, `explain`, `tighten`, `tldr`, `examples`, `diagram`, `glossary`, `bullets`, `steps`, `table`, `headers`, `fmtjson`, `markdown`.

Place the `type` line directly after the `prompt` line in each block. Example for the `critique` block:

```toml
[[shortcuts]]
name = 'critique'
description = 'Review as a draft spec and flag issues'
prompt = 'Review the text as if it were a draft spec or design doc. Output a structured critique with these sections: "Ambiguities" (statements that could be interpreted multiple ways), "Missing edge cases" (scenarios not addressed), "Hidden assumptions" (things presented as obvious that may not be), "Contradictions" (parts that conflict with each other). Quote the specific sentence under each finding. If a section has no findings, omit it.'
type = 'review'
```

And for a replace example (`tighten`):

```toml
[[shortcuts]]
name = 'tighten'
description = 'Cut filler; keep meaning; shorter'
prompt = 'Tighten the text. Cut filler, hedging, redundancy, and throat-clearing. Keep all substantive points. Do not add new information, do not change the meaning, do not change the document structure. Aim for roughly 60-70% of the original length.'
type = 'replace'
```

Do not change `name`, `description`, or `prompt` values. Do not add or remove blocks (there are 23).

- [ ] **Step 8: Run the shortcut tests to verify all pass**

Run: `go test -run 'TestResolveShortcutType|TestDefaultShortcuts_HaveResolvedTypes|TestLoadShortcuts_SeedsWhenMissing|TestSaveAndLoadShortcuts' .`
Expected: PASS (in particular `TestLoadShortcuts_SeedsWhenMissing` still expects 23 shortcuts — block count unchanged).

- [ ] **Step 9: Commit**

```bash
git add shortcuts.go shortcuts_test.go defaults/ai_shortcuts.toml
git commit -m "feat(shortcuts): add Type field with read-time resolution and typed defaults

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `inputPluginReview` mode, `reviewFocus`, and the review view

**Files:**
- Modify: `model.go` (inputMode enum lines 50-73; model struct AI shortcuts block ~145-156)
- Create: `plugin_review.go` (the review view + focus type)
- Test: Create `plugin_review_test.go`

- [ ] **Step 1: Write the failing test for `pluginReviewView`**

Create `plugin_review_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

func TestPluginReviewView_ShowsNoteAndReviewHeaders(t *testing.T) {
	left, right := newDiffViewports("the original note", "the AI review", 80, 10)
	out := pluginReviewView(left, right, reviewFocusReview, 80, 10)
	if !strings.Contains(out, "Note") {
		t.Errorf("review view missing 'Note' header:\n%s", out)
	}
	if !strings.Contains(out, "Review") {
		t.Errorf("review view missing 'Review' header:\n%s", out)
	}
	if strings.Contains(out, "Original") || strings.Contains(out, "New") {
		t.Errorf("review view must not reuse diff headers Original/New:\n%s", out)
	}
}

func TestPluginReviewView_FocusIsRenderable(t *testing.T) {
	left, right := newDiffViewports("a", "b", 80, 10)
	// Both focus states must render without panicking and produce output.
	if pluginReviewView(left, right, reviewFocusNote, 80, 10) == "" {
		t.Error("reviewFocusNote produced empty view")
	}
	if pluginReviewView(left, right, reviewFocusReview, 80, 10) == "" {
		t.Error("reviewFocusReview produced empty view")
	}
}

var _ = viewport.Model{}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestPluginReviewView .`
Expected: build failure — `undefined: pluginReviewView`, `undefined: reviewFocusReview`, `undefined: reviewFocusNote`.

- [ ] **Step 3: Add the `inputPluginReview` enum value**

In `model.go`, in the `const` block (lines 50-73), add `inputPluginReview` immediately after `inputPluginDiff`:

```go
	inputPluginDiff
	inputPluginReview
	inputNewFolder
```

- [ ] **Step 4: Add the `reviewFocus` field to the model**

In `model.go`, in the `// AI shortcuts` field block (after `activeShortcutProvider string`, around line 156), add:

```go
	reviewFocus reviewFocus // which pane scroll/keys act on in inputPluginReview
```

- [ ] **Step 5: Create `plugin_review.go`**

```go
package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type reviewFocus int

const (
	reviewFocusReview reviewFocus = iota // default: the AI review pane (right)
	reviewFocusNote                      // the original note pane (left)
)

var (
	reviewHeaderNoteStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)

	reviewHeaderReviewStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("75")).
				Padding(0, 1)

	reviewFocusedHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("75")).
				Padding(0, 1)

	reviewBorderStyle = lipgloss.NewStyle().
				BorderRight(true).
				BorderStyle(lipgloss.NormalBorder())
)

// pluginReviewView renders the read-only side-by-side review: the original
// note on the left, the AI-generated review on the right. The focused pane's
// header is highlighted.
func pluginReviewView(left, right viewport.Model, focus reviewFocus, width, height int) string {
	noteHeader := reviewHeaderNoteStyle.Render("── Note ──")
	reviewHeader := reviewHeaderReviewStyle.Render("── Review ──")
	if focus == reviewFocusNote {
		noteHeader = reviewFocusedHeaderStyle.Render("── Note ──")
	} else {
		reviewHeader = reviewFocusedHeaderStyle.Render("── Review ──")
	}

	halfWidth := width / 2

	leftPanel := reviewBorderStyle.Width(halfWidth).Height(height).Render(
		noteHeader + "\n" + left.View())

	rightPanel := lipgloss.NewStyle().Width(width - halfWidth - 1).Height(height).Render(
		reviewHeader + "\n" + right.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test -run TestPluginReviewView .`
Expected: PASS.

- [ ] **Step 7: Run the full build to confirm nothing else broke**

Run: `go build ./... && go test ./...`
Expected: PASS (no behaviour wired yet; enum + field + view compile clean).

- [ ] **Step 8: Commit**

```bash
git add model.go plugin_review.go plugin_review_test.go
git commit -m "feat(review): add inputPluginReview mode, reviewFocus, and review view

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Route shortcut runs by type; protect review from "No changes" dismissal

**Files:**
- Modify: `shortcuts_input.go` (the `"enter"` case of `handleShortcutSelect`, lines ~38-72)
- Modify: `model.go` (the `pluginDoneMsg` case, lines ~553-567)
- Test: `plugin_review_test.go` (append)

- [ ] **Step 1: Write the failing routing test**

Append to `plugin_review_test.go`:

```go
import (
	// add to the existing import block:
	tea "github.com/charmbracelet/bubbletea"
)

func runShortcutSelectWithType(t *testing.T, sc AIShortcut) model {
	t.Helper()
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("hello world")

	provider := defaultAIShortcutProvider
	plugin := pluginByName(m.plugins, provider)
	if plugin == nil {
		plugin = &fakePlugin{name: provider}
		m.plugins = []Plugin{plugin}
	}
	if err := savePluginConfig(provider, map[string]string{"api_key": "k", "model": "m"}); err != nil {
		t.Fatalf("savePluginConfig: %v", err)
	}
	m.shortcuts = []AIShortcut{sc}
	m.shortcutCursor = 0
	m.activeShortcutProvider = provider
	m.inputMode = inputShortcutSelect

	next, _ := m.handleShortcutSelect(pressEnter())
	return next.(model)
}

func TestShortcutSelect_ReviewType_EntersReviewMode(t *testing.T) {
	nm := runShortcutSelectWithType(t, AIShortcut{Name: "critique", Description: "d", Prompt: "p", Type: "review"})
	if nm.inputMode != inputPluginReview {
		t.Fatalf("inputMode = %v, want inputPluginReview", nm.inputMode)
	}
	if nm.reviewFocus != reviewFocusReview {
		t.Errorf("reviewFocus = %v, want reviewFocusReview (default)", nm.reviewFocus)
	}
}

func TestShortcutSelect_ReplaceType_EntersDiffMode(t *testing.T) {
	nm := runShortcutSelectWithType(t, AIShortcut{Name: "tighten", Description: "d", Prompt: "p", Type: "replace"})
	if nm.inputMode != inputPluginDiff {
		t.Fatalf("inputMode = %v, want inputPluginDiff", nm.inputMode)
	}
}

func TestShortcutSelect_InferredReview_EntersReviewMode(t *testing.T) {
	// No explicit Type -> resolved by name (critique => review).
	nm := runShortcutSelectWithType(t, AIShortcut{Name: "critique", Description: "d", Prompt: "p"})
	if nm.inputMode != inputPluginReview {
		t.Fatalf("inputMode = %v, want inputPluginReview", nm.inputMode)
	}
}

var _ = tea.KeyMsg{}
```

(If `tea` is already imported in `plugin_review_test.go` from Task 2, do not duplicate the import — keep one import block. The `var _ = tea.KeyMsg{}` line can be dropped if `tea` is otherwise used.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestShortcutSelect_ReviewType_EntersReviewMode|TestShortcutSelect_ReplaceType_EntersDiffMode|TestShortcutSelect_InferredReview' .`
Expected: FAIL — review shortcuts currently land in `inputPluginDiff`, so `TestShortcutSelect_ReviewType_EntersReviewMode` and `TestShortcutSelect_InferredReview_EntersReviewMode` fail.

- [ ] **Step 3: Branch the run dispatch by resolved type**

In `shortcuts_input.go`, in `handleShortcutSelect`, the `"enter"` case currently ends with:

```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginDiffResult = ""
		m.pluginProcessing = true
		ctx, cancel := context.WithCancel(context.Background())
		m.pluginCancel = cancel
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
		m.inputMode = inputPluginDiff
		chunks, errs := runShortcutStream(ctx, shortcut, content, provider, cfg)
		m.activeChunks = chunks
		return m, streamPluginCmd(chunks, errs)
```

Replace the `m.inputMode = inputPluginDiff` line with type-based routing:

```go
		content, onSelection := m.aiInputContent()
		m.aiRunOnSelection = onSelection
		m.pluginDiffOriginal = content
		m.pluginDiffResult = ""
		m.pluginProcessing = true
		ctx, cancel := context.WithCancel(context.Background())
		m.pluginCancel = cancel
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(content, "", m.editorWidth, m.editorHeight)
		if resolveShortcutType(shortcut) == "review" {
			m.inputMode = inputPluginReview
			m.reviewFocus = reviewFocusReview
		} else {
			m.inputMode = inputPluginDiff
		}
		chunks, errs := runShortcutStream(ctx, shortcut, content, provider, cfg)
		m.activeChunks = chunks
		return m, streamPluginCmd(chunks, errs)
```

- [ ] **Step 4: Protect review mode from the "No changes" auto-dismiss**

In `model.go`, the `pluginDoneMsg` case currently reads:

```go
	case pluginDoneMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale
		}
		m.pluginProcessing = false
		m.pluginCancel = nil
		m.activeChunks = nil
		if m.pluginDiffResult == m.pluginDiffOriginal || m.pluginDiffResult == "" {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			m.pluginDiffResult = ""
		}
		return m, nil
```

Replace the `if` block so review mode uses a different rule (an identical result is still readable; only an empty result closes):

```go
	case pluginDoneMsg:
		if msg.chunks != m.activeChunks {
			return m, nil // stale
		}
		m.pluginProcessing = false
		m.pluginCancel = nil
		m.activeChunks = nil
		if m.inputMode == inputPluginReview {
			if m.pluginDiffResult == "" {
				m.errMsg = "No review generated"
				m.inputMode = inputNone
				m.pluginActive = nil
				m.pluginDiffOriginal = ""
				m.pluginDiffResult = ""
			}
			return m, nil
		}
		if m.pluginDiffResult == m.pluginDiffOriginal || m.pluginDiffResult == "" {
			m.errMsg = "No changes"
			m.inputMode = inputNone
			m.pluginActive = nil
			m.pluginDiffOriginal = ""
			m.pluginDiffResult = ""
		}
		return m, nil
```

- [ ] **Step 5: Run the routing tests to verify they pass**

Run: `go test -run 'TestShortcutSelect_ReviewType_EntersReviewMode|TestShortcutSelect_ReplaceType_EntersDiffMode|TestShortcutSelect_InferredReview' .`
Expected: PASS.

- [ ] **Step 6: Run the existing shortcut/plugin tests to confirm no regression**

Run: `go test -run 'TestShortcutSelect|TestPluginPrompt|TestAIInputContent' .`
Expected: PASS (replace-path tests like `TestShortcutSelect_WithSelection_SendsOnlySelection` still pass — `{Name:"n"}` resolves to `replace`).

- [ ] **Step 7: Commit**

```bash
git add shortcuts_input.go model.go plugin_review_test.go
git commit -m "feat(review): route review shortcuts to inputPluginReview

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: `handlePluginReview` handler, dispatch, view wiring, status bar, resize

**Files:**
- Modify: `plugin_review.go` (add `handlePluginReview`)
- Modify: `model.go` (handler dispatch ~1013; resize block ~1870-1873; View dispatch ~1923-1924; status bar ~2026-2028)
- Test: `plugin_review_test.go` (append)

- [ ] **Step 1: Write the failing handler tests**

Append to `plugin_review_test.go`:

```go
func reviewModel(t *testing.T) model {
	t.Helper()
	m := newTestModel(t)
	setEditorSize(&m.editor, 80, 10)
	m.editor.SetValue("the untouched note")
	m.pluginDiffOriginal = "the untouched note"
	m.pluginDiffResult = "line one\nline two\nline three\nline four\nline five"
	m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
		m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	m.inputMode = inputPluginReview
	m.reviewFocus = reviewFocusReview
	m.pluginActive = &fakePlugin{name: "fake"}
	return m
}

func TestHandlePluginReview_TabTogglesFocus(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyTab})
	nm := next.(model)
	if nm.reviewFocus != reviewFocusNote {
		t.Fatalf("after Tab: reviewFocus = %v, want reviewFocusNote", nm.reviewFocus)
	}
	next2, _ := nm.handlePluginReview(tea.KeyMsg{Type: tea.KeyTab})
	if next2.(model).reviewFocus != reviewFocusReview {
		t.Errorf("after second Tab: want reviewFocusReview")
	}
}

func TestHandlePluginReview_EscClosesWithoutChangingNote(t *testing.T) {
	m := reviewModel(t)
	before := m.editor.Value()
	next, _ := m.handlePluginReview(pressEsc())
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.editor.Value() != before {
		t.Errorf("editor value changed: got %q, want %q", nm.editor.Value(), before)
	}
	if nm.pluginDiffResult != "" || nm.pluginActive != nil {
		t.Errorf("plugin state not cleared on close")
	}
}

func TestHandlePluginReview_QClosesWithoutChangingNote(t *testing.T) {
	m := reviewModel(t)
	before := m.editor.Value()
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.editor.Value() != before {
		t.Errorf("editor value changed on q-close")
	}
}

func TestHandlePluginReview_CopyWritesClipboard(t *testing.T) {
	m := reviewModel(t)
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	nm := next.(model)
	got, err := clipboardReadAllForTest()
	if err != nil {
		t.Skipf("clipboard unavailable in this environment: %v", err)
	}
	if got != m.pluginDiffResult {
		t.Errorf("clipboard = %q, want %q", got, m.pluginDiffResult)
	}
	if nm.inputMode != inputPluginReview {
		t.Errorf("copy should not close review; inputMode = %v", nm.inputMode)
	}
}

func TestHandlePluginReview_ScrollFocusedPane(t *testing.T) {
	m := reviewModel(t)
	// Force tiny viewport so content is scrollable.
	m.pluginDiffViewR = viewport.New(20, 1)
	m.pluginDiffViewR.SetContent("a\nb\nc\nd\ne")
	startOffset := m.pluginDiffViewR.YOffset
	next, _ := m.handlePluginReview(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	nm := next.(model)
	if nm.pluginDiffViewR.YOffset <= startOffset {
		t.Errorf("review pane did not scroll down: offset %d -> %d", startOffset, nm.pluginDiffViewR.YOffset)
	}
}
```

Add a tiny clipboard read helper at the bottom of `plugin_review_test.go`:

```go
func clipboardReadAllForTest() (string, error) {
	return clipboard.ReadAll()
}
```

Ensure the import block of `plugin_review_test.go` contains: `"strings"`, `"testing"`, `tea "github.com/charmbracelet/bubbletea"`, `"github.com/charmbracelet/bubbles/viewport"`, `"github.com/atotto/clipboard"`. Remove the earlier `var _ = viewport.Model{}` / `var _ = tea.KeyMsg{}` shims if the imports are now genuinely used.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -run TestHandlePluginReview .`
Expected: build failure — `undefined: (model).handlePluginReview`.

- [ ] **Step 3: Implement `handlePluginReview`**

Append to `plugin_review.go` (add `"github.com/atotto/clipboard"` and `tea "github.com/charmbracelet/bubbletea"` to its import block):

```go
func (m model) handlePluginReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if m.reviewFocus == reviewFocusReview {
			m.reviewFocus = reviewFocusNote
		} else {
			m.reviewFocus = reviewFocusReview
		}
		return m, nil
	case "up", "k":
		if m.reviewFocus == reviewFocusNote {
			m.pluginDiffViewL.LineUp(1)
		} else {
			m.pluginDiffViewR.LineUp(1)
		}
		return m, nil
	case "down", "j":
		if m.reviewFocus == reviewFocusNote {
			m.pluginDiffViewL.LineDown(1)
		} else {
			m.pluginDiffViewR.LineDown(1)
		}
		return m, nil
	case "c":
		_ = clipboard.WriteAll(m.pluginDiffResult)
		m.errMsg = "Review copied"
		return m, nil
	case "esc", "q":
		m.inputMode = inputNone
		m.pluginActive = nil
		m.pluginDiffOriginal = ""
		m.pluginDiffResult = ""
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}
```

- [ ] **Step 4: Wire the handler into the dispatch switch**

In `model.go`, after the `case inputPluginDiff:` / `return m.handlePluginDiff(msg)` pair (lines ~1013-1014), add:

```go
	case inputPluginReview:
		return m.handlePluginReview(msg)
```

- [ ] **Step 5: Wire the resize rebuild**

In `model.go`, the resize block reads:

```go
	if m.inputMode == inputPluginDiff {
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	}
```

Change the condition to also cover review:

```go
	if m.inputMode == inputPluginDiff || m.inputMode == inputPluginReview {
		m.pluginDiffViewL, m.pluginDiffViewR = newDiffViewports(
			m.pluginDiffOriginal, m.pluginDiffResult, m.editorWidth, m.editorHeight)
	}
```

- [ ] **Step 6: Wire the View dispatch**

In `model.go`, the View has:

```go
	} else if m.inputMode == inputPluginDiff {
		rightView = pluginDiffView(m.pluginDiffViewL, m.pluginDiffViewR, m.editorWidth, m.editorHeight)
```

Add a review branch immediately after that `else if` block (before the next `else if`):

```go
	} else if m.inputMode == inputPluginReview {
		rightView = pluginReviewView(m.pluginDiffViewL, m.pluginDiffViewR, m.reviewFocus, m.editorWidth, m.editorHeight)
```

- [ ] **Step 7: Wire the status bar**

In `model.go`, the status bar has:

```go
	} else if m.inputMode == inputPluginDiff {
		statusView = statusBarStyle.Width(m.width).Render(
			"Accept changes? (y/n)")
```

Add immediately after that block:

```go
	} else if m.inputMode == inputPluginReview {
		statusView = statusBarStyle.Width(m.width).Render(
			"Review — Tab:switch pane  c:copy  Esc:close")
```

- [ ] **Step 8: Run the handler tests to verify they pass**

Run: `go test -run TestHandlePluginReview .`
Expected: PASS (the clipboard test may `Skip` in a headless environment — that is acceptable).

- [ ] **Step 9: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add plugin_review.go model.go plugin_review_test.go
git commit -m "feat(review): handlePluginReview with tab/scroll/copy/close + view wiring

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Mouse wheel scroll in review mode

**Files:**
- Modify: `model.go` (the `tea.MouseMsg` case, lines ~583-593)
- Modify: `plugin_review.go` (add `handleReviewMouse`)
- Test: `plugin_review_test.go` (append)

- [ ] **Step 1: Write the failing mouse test**

Append to `plugin_review_test.go`:

```go
func TestHandleReviewMouse_WheelScrollsHoveredPane(t *testing.T) {
	m := reviewModel(t)
	m.treeWidth = 0 // editor area starts at x=0; mid = editorWidth/2 = 40
	m.pluginDiffViewL = viewport.New(20, 1)
	m.pluginDiffViewL.SetContent("a\nb\nc\nd\ne")
	m.pluginDiffViewR = viewport.New(20, 1)
	m.pluginDiffViewR.SetContent("v\nw\nx\ny\nz")

	// Wheel down over the left half (x=5) scrolls the note pane.
	leftStart := m.pluginDiffViewL.YOffset
	m2, _ := m.handleReviewMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, X: 5, Y: 3})
	nm := m2.(model)
	if nm.pluginDiffViewL.YOffset <= leftStart {
		t.Errorf("left pane did not scroll on wheel over left half")
	}

	// Wheel down over the right half (x=60) scrolls the review pane.
	rightStart := nm.pluginDiffViewR.YOffset
	m3, _ := nm.handleReviewMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, X: 60, Y: 3})
	nm2 := m3.(model)
	if nm2.pluginDiffViewR.YOffset <= rightStart {
		t.Errorf("right pane did not scroll on wheel over right half")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestHandleReviewMouse .`
Expected: build failure — `undefined: (model).handleReviewMouse`.

- [ ] **Step 3: Implement `handleReviewMouse`**

Append to `plugin_review.go`:

```go
// handleReviewMouse scrolls the pane the cursor is over when a wheel event
// arrives during review mode. X is absolute (the editor area begins at
// m.treeWidth); the split is at the editor's horizontal midpoint.
func (m model) handleReviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}
	localX := msg.X - m.treeWidth
	overLeft := localX < m.editorWidth/2
	const lines = 3
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if overLeft {
			m.pluginDiffViewL.LineUp(lines)
		} else {
			m.pluginDiffViewR.LineUp(lines)
		}
	case tea.MouseButtonWheelDown:
		if overLeft {
			m.pluginDiffViewL.LineDown(lines)
		} else {
			m.pluginDiffViewR.LineDown(lines)
		}
	}
	return m, nil
}
```

- [ ] **Step 4: Let wheel events reach review mode**

In `model.go`, the `tea.MouseMsg` case currently reads:

```go
	case tea.MouseMsg:
		if m.pluginProcessing {
			return m, nil
		}
		if m.inputMode == inputHelp {
			return handleMouseMsg(m, msg)
		}
		if m.inputMode != inputNone {
			return m, nil
		}
		return handleMouseMsg(m, msg)
```

Add a review branch before the `if m.inputMode != inputNone` guard:

```go
	case tea.MouseMsg:
		if m.pluginProcessing {
			return m, nil
		}
		if m.inputMode == inputHelp {
			return handleMouseMsg(m, msg)
		}
		if m.inputMode == inputPluginReview {
			return m.handleReviewMouse(msg)
		}
		if m.inputMode != inputNone {
			return m, nil
		}
		return handleMouseMsg(m, msg)
```

- [ ] **Step 5: Run the mouse test to verify it passes**

Run: `go test -run TestHandleReviewMouse .`
Expected: PASS.

- [ ] **Step 6: Run the full suite (mouse regression check)**

Run: `go test -run 'TestHandleReviewMouse|TestMouse|TestHandlePluginReview' .` then `go test ./...`
Expected: PASS — non-review mouse behaviour is unchanged (the new branch only triggers when `inputMode == inputPluginReview`).

- [ ] **Step 7: Commit**

```bash
git add plugin_review.go model.go plugin_review_test.go
git commit -m "feat(review): mouse wheel scrolls the hovered pane in review mode

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Type step in the shortcut create/edit flow

**Files:**
- Modify: `model.go` (inputMode enum: add `inputShortcutType`; handler dispatch ~1022-1023)
- Modify: `shortcuts_input.go` (move save from `handleShortcutPrompt` into a new `handleShortcutType`)
- Create: `shortcut_type_input.go` (the type selector view + state helpers)
- Modify: `model.go` (status bar OR View — render the selector; see Step 6)
- Test: `shortcuts_input_test.go` (append)

- [ ] **Step 1: Write the failing flow test**

Append to `shortcuts_input_test.go`:

```go
func TestShortcutFlow_Create_ReachesTypeStepAndPersistsType(t *testing.T) {
	m := newTestModel(t)
	m.shortcutEditing = -1
	m.inputMode = inputShortcutName
	m.shortcutNameInput.SetValue("mytest")

	n1, _ := m.handleShortcutName(pressEnter())
	m1 := n1.(model)
	m1.shortcutDescriptionInput.SetValue("desc")
	n2, _ := m1.handleShortcutDescription(pressEnter())
	m2 := n2.(model)
	m2.shortcutPromptInput.SetValue("do it")
	n3, _ := m2.handleShortcutPrompt(pressEnter())
	m3 := n3.(model)

	if m3.inputMode != inputShortcutType {
		t.Fatalf("after prompt: inputMode = %v, want inputShortcutType", m3.inputMode)
	}

	// Move selection to "review" and confirm.
	n4, _ := m3.handleShortcutType(tea.KeyMsg{Type: tea.KeyDown})
	m4 := n4.(model)
	n5, _ := m4.handleShortcutType(pressEnter())
	m5 := n5.(model)

	if m5.inputMode != inputNone {
		t.Fatalf("after type confirm: inputMode = %v, want inputNone", m5.inputMode)
	}
	if len(m5.shortcuts) != 1 {
		t.Fatalf("expected 1 saved shortcut, got %d", len(m5.shortcuts))
	}
	if m5.shortcuts[0].Type != "review" {
		t.Errorf("saved Type = %q, want review", m5.shortcuts[0].Type)
	}
	if m5.shortcuts[0].Name != "mytest" || m5.shortcuts[0].Prompt != "do it" {
		t.Errorf("saved shortcut fields wrong: %+v", m5.shortcuts[0])
	}
}

func TestShortcutFlow_Edit_PreselectsResolvedType(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{{Name: "critique", Description: "d", Prompt: "p"}} // no Type -> infers review
	m.shortcutCursor = 0
	m.shortcutEditing = 0
	m.shortcutTempName = "critique"
	m.shortcutTempDescription = "d"
	m.inputMode = inputShortcutPrompt
	m.shortcutPromptInput.SetValue("p")

	n, _ := m.handleShortcutPrompt(pressEnter())
	nm := n.(model)
	if nm.inputMode != inputShortcutType {
		t.Fatalf("inputMode = %v, want inputShortcutType", nm.inputMode)
	}
	if nm.shortcutTypeCursor != shortcutTypeIndex("review") {
		t.Errorf("type cursor = %d, want review index for an inferred-review shortcut", nm.shortcutTypeCursor)
	}
}

func TestShortcutFlow_Type_EscCancels(t *testing.T) {
	m := newTestModel(t)
	m.shortcutEditing = -1
	m.inputMode = inputShortcutType
	m.shortcutTempName = "x"
	m.shortcutTempDescription = "y"
	m.shortcutTempPrompt = "z"
	n, _ := m.handleShortcutType(pressEsc())
	nm := n.(model)
	if nm.inputMode != inputNone {
		t.Errorf("Esc should cancel to inputNone, got %v", nm.inputMode)
	}
	if len(nm.shortcuts) != 0 {
		t.Errorf("Esc must not save; got %d shortcuts", len(nm.shortcuts))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run 'TestShortcutFlow_Create_ReachesTypeStep|TestShortcutFlow_Edit_PreselectsResolvedType|TestShortcutFlow_Type_EscCancels' .`
Expected: build failure — `undefined: inputShortcutType`, `undefined: (model).handleShortcutType`, `undefined: shortcutTypeIndex`, `undefined field shortcutTypeCursor`, `undefined field shortcutTempPrompt`.

- [ ] **Step 3: Add enum value and model fields**

In `model.go` const block, add `inputShortcutType` immediately after `inputShortcutPrompt`:

```go
	inputShortcutPrompt
	inputShortcutType
	inputShortcutDeleteConfirm
```

In `model.go`, in the `// AI shortcuts` field block, add next to the other `shortcutTemp*` fields:

```go
	shortcutTempPrompt string
	shortcutTypeCursor int
```

- [ ] **Step 4: Create `shortcut_type_input.go`**

```go
package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// shortcutTypeOptions is the ordered list of selectable types.
var shortcutTypeOptions = []string{"replace", "review"}

// shortcutTypeIndex returns the cursor index for a type string, defaulting
// to 0 ("replace") for anything unrecognised.
func shortcutTypeIndex(t string) int {
	for i, o := range shortcutTypeOptions {
		if o == t {
			return i
		}
	}
	return 0
}

func (m model) handleShortcutType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.shortcutTypeCursor > 0 {
			m.shortcutTypeCursor--
		}
		return m, nil
	case "down", "j":
		if m.shortcutTypeCursor < len(shortcutTypeOptions)-1 {
			m.shortcutTypeCursor++
		}
		return m, nil
	case "r":
		m.shortcutTypeCursor = shortcutTypeIndex("replace")
		return m, nil
	case "v":
		m.shortcutTypeCursor = shortcutTypeIndex("review")
		return m, nil
	case "enter":
		shortcut := AIShortcut{
			Name:        m.shortcutTempName,
			Description: m.shortcutTempDescription,
			Prompt:      m.shortcutTempPrompt,
			Type:        shortcutTypeOptions[m.shortcutTypeCursor],
		}
		if m.shortcutEditing >= 0 {
			m.shortcuts[m.shortcutEditing] = shortcut
		} else {
			m.shortcuts = append(m.shortcuts, shortcut)
		}
		if err := saveShortcuts(m.shortcuts); err != nil {
			m.errMsg = "Failed to save shortcuts: " + err.Error()
		}
		m.inputMode = inputNone
		m.shortcutEditing = -1
		return m, nil
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
		return m, nil
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

var (
	shortcutTypeTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	shortcutTypeCursorStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("117")).
				Bold(true)
	shortcutTypeItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	shortcutTypeHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

var shortcutTypeHelp = map[string]string{
	"replace": "Replace the note/selection with the AI output (diff + accept)",
	"review":  "Show the AI output side-by-side, read-only (note unchanged)",
}

func shortcutTypeSelectorView(cursor int, width, height int) string {
	var b strings.Builder
	b.WriteString(shortcutTypeTitleStyle.Render("Shortcut type") + "\n\n")
	for i, o := range shortcutTypeOptions {
		line := "  " + o + "  — " + shortcutTypeHelp[o]
		if i == cursor {
			b.WriteString(shortcutTypeCursorStyle.Render("> "+o) +
				shortcutTypeItemStyle.Render("  — "+shortcutTypeHelp[o]) + "\n")
		} else {
			b.WriteString(shortcutTypeItemStyle.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + shortcutTypeHintStyle.Render("↑/↓ or r/v select  Enter:save  Esc:cancel"))
	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(b.String())
}
```

- [ ] **Step 5: Route prompt → type instead of prompt → save**

In `shortcuts_input.go`, `handleShortcutPrompt`'s `"enter"` case currently is:

```go
	case "enter":
		prompt := m.shortcutPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		shortcut := AIShortcut{
			Name:        m.shortcutTempName,
			Description: m.shortcutTempDescription,
			Prompt:      prompt,
		}
		if m.shortcutEditing >= 0 {
			m.shortcuts[m.shortcutEditing] = shortcut
		} else {
			m.shortcuts = append(m.shortcuts, shortcut)
		}
		if err := saveShortcuts(m.shortcuts); err != nil {
			m.errMsg = "Failed to save shortcuts: " + err.Error()
		}
		m.inputMode = inputNone
		m.shortcutEditing = -1
```

Replace that whole `case "enter":` body with a transition to the type step:

```go
	case "enter":
		prompt := m.shortcutPromptInput.Value()
		if prompt == "" {
			return m, nil
		}
		m.shortcutTempPrompt = prompt
		if m.shortcutEditing >= 0 {
			m.shortcutTypeCursor = shortcutTypeIndex(resolveShortcutType(m.shortcuts[m.shortcutEditing]))
		} else {
			m.shortcutTypeCursor = shortcutTypeIndex("replace")
		}
		m.inputMode = inputShortcutType
		return m, nil
```

(Leave the `"esc"`, `"ctrl+q"`, and the trailing `m.shortcutPromptInput, cmd = ...` lines of `handleShortcutPrompt` unchanged.)

- [ ] **Step 6: Wire dispatch, View, and status bar for `inputShortcutType`**

In `model.go` handler dispatch, after the `case inputShortcutPrompt:` / `return m.handleShortcutPrompt(msg)` pair, add:

```go
	case inputShortcutType:
		return m.handleShortcutType(msg)
```

In `model.go` View, next to the `inputShortcutSelect` rendering branch:

```go
	} else if m.inputMode == inputShortcutSelect {
		rightView = shortcutSelectorView(m.shortcuts, m.shortcutCursor, m.activeShortcutProvider, m.editorWidth, m.editorHeight)
```

add immediately after it:

```go
	} else if m.inputMode == inputShortcutType {
		rightView = shortcutTypeSelectorView(m.shortcutTypeCursor, m.editorWidth, m.editorHeight)
```

(The name/description/prompt steps render via the status bar text input; the type step is a selector, so it renders in the editor area like `inputShortcutSelect`. No status-bar branch is needed for `inputShortcutType` — the default editor-area render path handles it.)

- [ ] **Step 7: Run the flow tests to verify they pass**

Run: `go test -run 'TestShortcutFlow_Create_ReachesTypeStep|TestShortcutFlow_Edit_PreselectsResolvedType|TestShortcutFlow_Type_EscCancels' .`
Expected: PASS.

- [ ] **Step 8: Update the existing create-flow test that asserted the old terminal step**

`shortcuts_input_test.go` contains `TestShortcutFlow_Create_GoesThroughDescriptionStep`, whose final assertions expect `handleShortcutPrompt(pressEnter())` to land in `inputNone` and save. It must now expect `inputShortcutType`. Locate the block after `nm3.shortcutPromptInput.SetValue("do the thing")` and replace the post-prompt assertions so they read:

```go
	nm3.shortcutPromptInput.SetValue("do the thing")
	next4, _ := nm3.handleShortcutPrompt(pressEnter())
	nm4 := next4.(model)
	if nm4.inputMode != inputShortcutType {
		t.Fatalf("after prompt: inputMode = %v, want inputShortcutType", nm4.inputMode)
	}
	if nm4.shortcutTempPrompt != "do the thing" {
		t.Errorf("shortcutTempPrompt = %q, want %q", nm4.shortcutTempPrompt, "do the thing")
	}
	next5, _ := nm4.handleShortcutType(pressEnter())
	nm5 := next5.(model)
	if nm5.inputMode != inputNone {
		t.Fatalf("after type: inputMode = %v, want inputNone", nm5.inputMode)
	}
	if len(nm5.shortcuts) == 0 || nm5.shortcuts[len(nm5.shortcuts)-1].Name != "mytest" {
		t.Errorf("shortcut not saved after type step: %+v", nm5.shortcuts)
	}
```

If the original test asserted specific fields after save (e.g. prompt value), keep those assertions but read them from `nm5` instead of `nm4`. Inspect the actual file contents before editing and adapt the surrounding variable names if they differ.

- [ ] **Step 9: Run the full shortcut input suite**

Run: `go test -run 'TestShortcutFlow|TestShortcutSelect' .` then `go test ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add model.go shortcuts_input.go shortcut_type_input.go shortcuts_input_test.go
git commit -m "feat(shortcuts): add type selection step to create/edit flow

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Documentation — help modal and README

**Files:**
- Modify: `help_modal.go` (the "Shortcut Picker" and "Diff View" sections, lines ~74-94)
- Modify: `README.md` (the Features list / plugin description)
- Test: `help_modal_test.go` (append)

- [ ] **Step 1: Write the failing help-content test**

Append to `help_modal_test.go`:

```go
func TestHelpContent_IncludesReviewMode(t *testing.T) {
	out := helpContent(80)
	if !strings.Contains(out, "Review") {
		t.Errorf("help content missing Review section/keys:\n%s", out)
	}
	if !strings.Contains(out, "switch pane") && !strings.Contains(out, "Switch pane") {
		t.Errorf("help content missing review Tab/switch-pane key:\n%s", out)
	}
}
```

Ensure `help_modal_test.go` imports `"strings"` and `"testing"` (add `"strings"` if absent).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestHelpContent_IncludesReviewMode .`
Expected: FAIL — no "Review" entry in help yet.

- [ ] **Step 3: Add a "Review View" section to the help modal**

In `help_modal.go`, the section list contains a "Diff View" block:

```go
	{
		title: "Diff View",
		entries: []helpEntry{
			{"y", "Accept changes"},
			{"n", "Reject changes"},
		},
	},
```

Add a new section immediately after the "Diff View" block:

```go
	{
		title: "Review View",
		entries: []helpEntry{
			{"Tab", "Switch pane (Note / Review)"},
			{"↑ / ↓ / j / k", "Scroll focused pane"},
			{"Mouse wheel", "Scroll hovered pane"},
			{"c", "Copy review to clipboard"},
			{"Esc / q", "Close (note unchanged)"},
		},
	},
```

- [ ] **Step 4: Run the help test to verify it passes**

Run: `go test -run TestHelpContent_IncludesReviewMode .`
Expected: PASS.

- [ ] **Step 5: Update the README**

In `README.md`, the Features list has:

```
- **Plugin system** with blackbox.ai and OpenRouter integrations for LLM-powered note transformation (rephrase, translate, redraft)
```

and

```
- **Side-by-side diff view** for reviewing plugin changes
```

Replace those two bullets with:

```
- **Plugin system** with blackbox.ai and OpenRouter integrations for LLM-powered note transformation (rephrase, translate, redraft)
- **AI shortcuts** with two modes per shortcut: *replace* (diff + accept) or *review* (read-only side-by-side commentary that never edits the note)
- **Side-by-side diff view** for reviewing plugin changes, and a read-only review view (Tab to switch panes, mouse/keys to scroll, `c` to copy)
```

If a "Usage" or "Shortcuts" section in the README documents `Ctrl+G`, add a sentence there: "Each AI shortcut has a type — `replace` rewrites the note via the diff+accept flow; `review` opens a read-only side-by-side pane you can scroll and copy from. When creating a shortcut you choose its type as the final step." If no such section exists, the Features bullets suffice.

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add help_modal.go help_modal_test.go README.md
git commit -m "docs: document AI shortcut review mode in help and README

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Final Verification

- [ ] **Run the entire test suite and build**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: all PASS, no vet warnings introduced by the new files.

- [ ] **Manual smoke (optional, if a terminal is available)**

Build `go build -o clipad .`, open a note, `Ctrl+G`, run `critique` → expect a read-only side-by-side "Note / Review" view; Tab switches the highlighted pane; `j/k` and mouse wheel scroll; `c` copies; `Esc` closes leaving the note unchanged. Run `tighten` → expect the unchanged diff + "Accept changes? (y/n)" flow.
