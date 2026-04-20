# AI Shortcut Descriptions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a required `Description` field to each AI shortcut and show it next to the name in the Ctrl+G selector modal so users can tell at a glance what a shortcut does.

**Architecture:** Extend the `AIShortcut` struct with a `Description` field persisted in TOML. Insert a new input step (`inputShortcutDescription`) between name and prompt in the create/edit flow. Change `shortcutSelectorView` from a one-line-per-item list to a two-column layout (name padded to the longest name, then `—`, then dim description, truncated at modal width). Seed the 23 bundled defaults with descriptions so first-run users get the full experience.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, `github.com/pelletier/go-toml/v2`, `embed`.

**Spec:** `docs/superpowers/specs/2026-04-21-shortcut-descriptions-design.md`

---

## File Structure

**Modified files**
- `shortcuts.go` — `AIShortcut` struct gains `Description string` field.
- `defaults/ai_shortcuts.toml` — each of the 23 entries gains a `description = '…'` line.
- `shortcuts_test.go` — round-trip covers description; default-seeding asserts all descriptions non-empty.
- `shortcuts_modal.go` — new two-column rendering with truncation and dim styling.
- `shortcuts_input.go` — name handler advances to description step; new `handleShortcutDescription`; prompt handler builds shortcut with description; `e` prefill path updated.
- `model.go` — new `inputShortcutDescription` constant, new `shortcutDescriptionInput` + `shortcutTempDescription` fields, initialization, router case, status-bar case.

**New test files**
- `shortcuts_modal_test.go` — render-based tests for `shortcutSelectorView`.
- Extended `shortcuts_input_test.go` would be new; we'll create it fresh (no existing file).

---

### Task 1: Add `Description` field to `AIShortcut` (TDD)

**Files:**
- Modify: `shortcuts.go` (the struct at lines 16-19)
- Test: `shortcuts_test.go` (the existing `TestSaveAndLoadShortcuts` at lines 11-36)

- [ ] **Step 1: Extend the round-trip test with a description field**

Replace the body of `TestSaveAndLoadShortcuts` in `shortcuts_test.go` with:

```go
func TestSaveAndLoadShortcuts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	shortcuts := []AIShortcut{
		{Name: "Fix grammar", Description: "Correct grammar errors", Prompt: "Fix grammar errors"},
		{Name: "Summarize", Description: "Short summary", Prompt: "Summarize this text"},
	}
	if err := saveShortcuts(shortcuts); err != nil {
		t.Fatalf("saveShortcuts() error: %v", err)
	}

	loaded, err := loadShortcuts()
	if err != nil {
		t.Fatalf("loadShortcuts() error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d shortcuts, want 2", len(loaded))
	}
	if loaded[0].Name != "Fix grammar" {
		t.Errorf("first shortcut name = %q, want %q", loaded[0].Name, "Fix grammar")
	}
	if loaded[0].Description != "Correct grammar errors" {
		t.Errorf("first shortcut description = %q, want %q", loaded[0].Description, "Correct grammar errors")
	}
	if loaded[1].Prompt != "Summarize this text" {
		t.Errorf("second shortcut prompt = %q, want %q", loaded[1].Prompt, "Summarize this text")
	}
}
```

- [ ] **Step 2: Run the test; verify it fails to compile**

Run: `go test ./... -run TestSaveAndLoadShortcuts`
Expected: build error — `unknown field Description in struct literal of type AIShortcut`.

- [ ] **Step 3: Add the `Description` field to `AIShortcut`**

In `shortcuts.go`, replace the struct definition (around line 16) with:

```go
type AIShortcut struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`
}
```

- [ ] **Step 4: Run the test; verify it passes**

Run: `go test ./... -run TestSaveAndLoadShortcuts -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite to catch any breakage**

Run: `go test ./...`
Expected: PASS. Seeded-default tests may still pass (they don't check description yet).

- [ ] **Step 6: Commit**

```bash
git add shortcuts.go shortcuts_test.go
git commit -m "feat(shortcuts): add Description field to AIShortcut"
```

---

### Task 2: Seed descriptions in `defaults/ai_shortcuts.toml`

**Files:**
- Modify: `defaults/ai_shortcuts.toml`
- Test: `shortcuts_test.go` (extend `TestDefaultShortcutsEmbeddedTOMLParses`)

- [ ] **Step 1: Extend the embedded-defaults test to require non-empty descriptions**

In `shortcuts_test.go`, locate `TestDefaultShortcutsEmbeddedTOMLParses` (around lines 137-162). Inside the `for i, n := range want` loop, add a description check alongside the existing prompt check:

```go
	for i, n := range want {
		if cfg.Shortcuts[i].Name != n {
			t.Errorf("shortcut %d: want name %q, got %q", i, n, cfg.Shortcuts[i].Name)
		}
		if cfg.Shortcuts[i].Prompt == "" {
			t.Errorf("shortcut %q: empty prompt", n)
		}
		if cfg.Shortcuts[i].Description == "" {
			t.Errorf("shortcut %q: empty description", n)
		}
	}
```

- [ ] **Step 2: Run the test; verify it fails**

Run: `go test ./... -run TestDefaultShortcutsEmbeddedTOMLParses -v`
Expected: FAIL — each of the 23 shortcuts reports "empty description".

- [ ] **Step 3: Add `description` to every entry in `defaults/ai_shortcuts.toml`**

Replace the contents of `defaults/ai_shortcuts.toml` with:

```toml
[[shortcuts]]
name = 'prd'
description = 'Turn text into a PRD with TBDs for gaps'
prompt = 'Turn the text into professional product requirements document. Add TBDs for things that are not clear but relevant in your view to have a complete and full spec'

[[shortcuts]]
name = 'userstory'
description = 'Rewrite as user stories with acceptance criteria'
prompt = 'Convert the text into one or more user stories in the format "As a <role>, I want <capability>, so that <benefit>." Below each story add 3-6 acceptance criteria as a bulleted list. Group related stories under a short heading. Rewrite freely for clarity.'

[[shortcuts]]
name = 'acceptance'
description = 'Write Gherkin acceptance scenarios'
prompt = 'Extract or write acceptance criteria for the feature described in the text. Output as Gherkin scenarios using "Scenario:", "Given", "When", "Then" (and "And" for additional steps). Cover the happy path plus important edge cases and error conditions. If the text is too vague to derive a scenario, list missing information under an "Open questions" heading at the end.'

[[shortcuts]]
name = 'critique'
description = 'Review as a draft spec and flag issues'
prompt = 'Review the text as if it were a draft spec or design doc. Output a structured critique with these sections: "Ambiguities" (statements that could be interpreted multiple ways), "Missing edge cases" (scenarios not addressed), "Hidden assumptions" (things presented as obvious that may not be), "Contradictions" (parts that conflict with each other). Quote the specific sentence under each finding. If a section has no findings, omit it.'

[[shortcuts]]
name = 'todos'
description = 'Extract actionable items as a checkbox list'
prompt = 'Extract every actionable item from the text and output as a markdown checkbox list (- [ ] item). Phrase each item as a concrete verb-led action. Group related items under short bold headings if there are obvious clusters. Drop items that are observations, decisions already made, or aspirations without an action — only true todos.'

[[shortcuts]]
name = 'prioritize'
description = 'Re-rank todos into Now / Next / Later'
prompt = 'Take the todo items in the text and re-rank them by impact and effort. Output three sections: "## Now" (high-impact, ready to start), "## Next" (important but blocked or lower urgency), "## Later" (nice-to-have or low-impact). Within each section use a markdown checkbox list. After each item, append a short " — <reason>" explaining the placement. Drop duplicates and merge near-duplicates.'

[[shortcuts]]
name = 'breakdown'
description = 'Decompose a goal into nested subtasks'
prompt = 'Take the high-level task or goal in the text and decompose it into concrete subtasks. Output as a hierarchical markdown checkbox list, with sub-items nested under their parent. Each leaf should be small enough to complete in one sitting. Add a short "## Open questions" section at the end for anything that needs to be resolved before starting.'

[[shortcuts]]
name = 'onboard'
description = 'Rewrite as an onboarding doc for new engineers'
prompt = 'Restructure the text as an onboarding document for an engineer who has never seen this system. Use these sections in order: "## What it is" (one paragraph), "## Why it exists" (problem it solves), "## Mental model" (the core concepts and how they relate), "## How to use it" (the typical workflow), "## Where the code lives" (key files/modules if mentioned), "## Common gotchas" (pitfalls to avoid). Rewrite freely; assume the reader has solid general engineering background but zero context on this project.'

[[shortcuts]]
name = 'explain'
description = 'Rewrite as a clear ground-up explainer'
prompt = 'Restructure the text as a clean explainer of how the system works. Open with a one-paragraph summary. Then build up understanding from the ground up: introduce concepts before they are used, show the simple case before complications, and end with a section on edge cases or surprising behavior. Use short sections with descriptive headings. Add concrete examples inline. Rewrite freely for clarity.'

[[shortcuts]]
name = 'tighten'
description = 'Cut filler; keep meaning; shorter'
prompt = 'Tighten the text. Cut filler, hedging, redundancy, and throat-clearing. Keep all substantive points. Do not add new information, do not change the meaning, do not change the document structure. Aim for roughly 60-70% of the original length.'

[[shortcuts]]
name = 'tldr'
description = 'Add a TL;DR at the top'
prompt = 'Add a "## TL;DR" section at the very top of the document containing 1-3 sentences (or a short bullet list if the content is heterogeneous) that capture the key takeaway. Keep the rest of the document unchanged.'

[[shortcuts]]
name = 'outline'
description = 'Produce a nested outline of topics'
prompt = 'Produce a hierarchical outline of the text as a nested markdown bullet list. Each bullet should be a short noun phrase or sentence fragment naming a topic, not a summary. Mirror the logical structure even if the source is not well-organized.'

[[shortcuts]]
name = 'questions'
description = 'List open questions and TBDs'
prompt = 'Extract every open question, uncertainty, "TBD", or follow-up implied by the text. Output as a markdown bullet list under "## Open questions". For each, add a short " — <context>" pointing to where it came from. Include both explicit questions and ones clearly implied by gaps in the text.'

[[shortcuts]]
name = 'examples'
description = 'Add concrete examples inline after claims'
prompt = 'Find abstract or generic claims in the text and add concrete examples that illustrate them. Insert each example inline, immediately after the claim it illustrates, prefixed with "Example:" on its own line. Keep all original wording. Do not invent facts about the system being described; if you cannot ground an example in the source, use a generic but realistic scenario.'

[[shortcuts]]
name = 'diagram'
description = 'Insert Mermaid diagrams where they help'
prompt = 'Identify parts of the text that would be clearer with a diagram (sequence of steps, state machine, component relationships, data flow). For each, insert a Mermaid code block at the appropriate spot in the text, choosing the right diagram type (flowchart, sequenceDiagram, stateDiagram, classDiagram). Keep the surrounding prose intact. If nothing in the text benefits from a diagram, return the text unchanged.'

[[shortcuts]]
name = 'glossary'
description = 'Add a glossary of domain terms at the end'
prompt = 'Identify domain-specific terms, jargon, and acronyms used in the text. Add a "## Glossary" section at the end with each term as a bold entry followed by a one-sentence plain-language definition. Order alphabetically. Do not modify the original text.'

[[shortcuts]]
name = 'risks'
description = 'List risks, gotchas, and failure modes'
prompt = 'Extract risks, pitfalls, edge cases, failure modes, and gotchas from the text — both ones explicitly mentioned and ones a careful reader would infer. Output as a markdown bullet list under "## Risks & gotchas". For each, add a short note on what could trigger it and, if obvious, how to mitigate.'

[[shortcuts]]
name = 'bullets'
description = 'Convert prose into a bullet list'
prompt = 'Convert the text into a markdown bullet list. Split by topic or item — one bullet per discrete point. Do not rewrite content beyond minor cleanup needed to make each item read well as a standalone bullet. Preserve order. If the source already uses bullets, only fix formatting (indentation, marker consistency).'

[[shortcuts]]
name = 'steps'
description = 'Convert into a numbered step list'
prompt = 'Convert the text into a numbered markdown list of sequential steps (1., 2., 3., …). Use this when the content describes an ordered procedure. Each step starts with an imperative verb. Preserve order and original wording where possible; tighten only for clarity. If a step has substeps, nest them as a bullet sublist.'

[[shortcuts]]
name = 'table'
description = 'Convert parallel structure into a table'
prompt = 'Convert the text into a markdown table when the content has parallel structure (multiple items sharing the same attributes). Choose columns from the attributes that appear repeatedly. Emit rows in source order. Right-align numeric columns. If the content does not lend itself to a table, return the text unchanged.'

[[shortcuts]]
name = 'headers'
description = 'Insert section headers by topic'
prompt = 'Add markdown headers (## and ###) to organize the text by topic. Do not rewrite paragraphs; only insert section headers and small whitespace fixes. Use sentence case. Choose header levels that reflect logical hierarchy. Do not add a top-level # title unless the text clearly lacks one.'

[[shortcuts]]
name = 'fmtjson'
description = 'Pretty-print JSON blocks in the text'
prompt = 'Pretty-print every JSON object or array in the text with 2-space indentation and sorted keys where the source order is not meaningful. Preserve everything else verbatim. If a fenced code block contains JSON but lacks the json language tag, add it. If a JSON-looking blob is loose in the text, wrap it in a fenced code block tagged json.'

[[shortcuts]]
name = 'markdown'
description = 'Clean up markdown formatting only'
prompt = 'Clean up the markdown formatting of the text without changing any content: fix list indentation, normalize bullet and number markers, fix heading levels, close unclosed code fences, escape stray characters, fix broken link syntax, collapse consecutive blank lines. Do not rewrite, summarize, or remove anything.'
```

- [ ] **Step 4: Run the test; verify it passes**

Run: `go test ./... -run TestDefaultShortcutsEmbeddedTOMLParses -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add defaults/ai_shortcuts.toml shortcuts_test.go
git commit -m "feat(shortcuts): seed descriptions for default shortcuts"
```

---

### Task 3: Two-column selector rendering with dim descriptions (TDD)

**Files:**
- Modify: `shortcuts_modal.go`
- Create: `shortcuts_modal_test.go`

- [ ] **Step 1: Write failing render tests**

Create `shortcuts_modal_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestShortcutSelectorView_ShowsDescriptions(t *testing.T) {
	shortcuts := []AIShortcut{
		{Name: "prd", Description: "Turn text into a PRD with TBDs for gaps"},
		{Name: "tldr", Description: "Add a TL;DR at the top"},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 120, 20)
	if !strings.Contains(out, "prd") {
		t.Error("missing shortcut name 'prd'")
	}
	if !strings.Contains(out, "Turn text into a PRD with TBDs for gaps") {
		t.Error("missing description text for 'prd'")
	}
	if !strings.Contains(out, "Add a TL;DR at the top") {
		t.Error("missing description text for 'tldr'")
	}
	if !strings.Contains(out, "—") {
		t.Error("missing em-dash separator between name and description")
	}
}

func TestShortcutSelectorView_NamesAlignToLongest(t *testing.T) {
	shortcuts := []AIShortcut{
		{Name: "a", Description: "first"},
		{Name: "longname", Description: "second"},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 120, 20)
	// Each description should sit at the same column across rows.
	// Find the column index of each em-dash; they must match.
	lines := strings.Split(out, "\n")
	var dashCols []int
	for _, ln := range lines {
		if idx := strings.Index(ln, "—"); idx >= 0 {
			dashCols = append(dashCols, idx)
		}
	}
	if len(dashCols) < 2 {
		t.Fatalf("expected at least 2 em-dash lines, got %d in:\n%s", len(dashCols), out)
	}
	for i := 1; i < len(dashCols); i++ {
		if dashCols[i] != dashCols[0] {
			t.Errorf("em-dash columns not aligned: %v", dashCols)
		}
	}
}

func TestShortcutSelectorView_EmptyDescriptionFallsBackToNameOnly(t *testing.T) {
	shortcuts := []AIShortcut{
		{Name: "bare", Description: ""},
		{Name: "full", Description: "has a description"},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 120, 20)
	// The 'bare' line should not contain an em-dash.
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "bare") && strings.Contains(ln, "—") {
			t.Errorf("empty-description row should not have em-dash: %q", ln)
		}
	}
	// The 'full' line should still have it.
	foundFull := false
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "full") && strings.Contains(ln, "—") {
			foundFull = true
		}
	}
	if !foundFull {
		t.Error("row with non-empty description is missing em-dash")
	}
}

func TestShortcutSelectorView_TruncatesLongDescription(t *testing.T) {
	longDesc := strings.Repeat("x", 500)
	shortcuts := []AIShortcut{
		{Name: "a", Description: longDesc},
	}
	out := shortcutSelectorView(shortcuts, 0, "blackbox", 30, 20)
	if !strings.Contains(out, "…") {
		t.Error("expected ellipsis indicating truncation")
	}
	// Verify no rendered line is wildly wider than the modal width.
	for _, ln := range strings.Split(out, "\n") {
		if len(ln) > 200 { // rough sanity bound (styled output has escape codes)
			t.Errorf("line appears untruncated at narrow width (len=%d): %q", len(ln), ln)
		}
	}
}

func TestShortcutSelectorView_EmptyListUnchanged(t *testing.T) {
	out := shortcutSelectorView(nil, 0, "blackbox", 80, 10)
	if !strings.Contains(out, "No shortcuts") {
		t.Errorf("empty-list rendering changed: %q", out)
	}
}
```

- [ ] **Step 2: Run the test; verify it fails**

Run: `go test ./... -run TestShortcutSelectorView -v`
Expected: FAIL — descriptions not rendered, em-dash absent, alignment tests fail.

- [ ] **Step 3: Replace the selector view with the new layout**

In `shortcuts_modal.go`, replace the contents of the file with:

```go
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	shortcutItemStyle = lipgloss.NewStyle().
		PaddingLeft(2)

	shortcutCursorStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("117")).
		Bold(true)

	shortcutEmptyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		PaddingLeft(2)

	shortcutHintStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		PaddingLeft(2)

	shortcutDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
)

// truncateRight shortens s so its rune length is at most max, appending an
// ellipsis when truncation happens. Returns s unchanged if short enough or if
// max is too small to hold anything meaningful.
func truncateRight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

func shortcutSelectorView(shortcuts []AIShortcut, cursor int, provider string, width, height int) string {
	if len(shortcuts) == 0 {
		content := shortcutEmptyStyle.Render("No shortcuts. Press Ctrl+L to create one.")
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1).
			Render(content)
	}

	maxName := 0
	for _, s := range shortcuts {
		if n := len([]rune(s.Name)); n > maxName {
			maxName = n
		}
	}
	nameCol := maxName + 2

	// Budget for description: modal width minus outer padding (Padding 0,1 = 2)
	// minus the cursor/item prefix ("> " / "  ", each length 2), minus nameCol,
	// minus the " — " separator (3 runes).
	descBudget := width - 2 - 2 - nameCol - 3
	if descBudget < 0 {
		descBudget = 0
	}

	var rows []string
	for i, s := range shortcuts {
		namePart := s.Name + strings.Repeat(" ", nameCol-len([]rune(s.Name)))
		var line string
		if i == cursor {
			line = shortcutCursorStyle.Render("> " + namePart)
		} else {
			line = shortcutItemStyle.Render("  " + namePart)
		}
		if s.Description != "" {
			desc := truncateRight(s.Description, descBudget)
			if desc != "" {
				line += shortcutDescStyle.Render("— " + desc)
			}
		}
		rows = append(rows, line)
	}

	items := strings.Join(rows, "\n")
	providerLine := shortcutHintStyle.Render("Provider: " + provider + "  (p:cycle)")
	hint := shortcutHintStyle.Render("Enter:run  e:edit  d:delete  Esc:close")
	content := items + "\n" + providerLine + "\n" + hint

	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(content)
}
```

- [ ] **Step 4: Run the tests; verify they pass**

Run: `go test ./... -run TestShortcutSelectorView -v`
Expected: PASS for all five tests.

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add shortcuts_modal.go shortcuts_modal_test.go
git commit -m "feat(shortcuts): show descriptions in Ctrl+G selector"
```

---

### Task 4: Model fields, input-mode constant, status bar

**Files:**
- Modify: `model.go`

- [ ] **Step 1: Add the `inputShortcutDescription` constant**

In `model.go`, locate the input-mode block (around lines 55-58) that currently reads:

```go
	inputShortcutSelect
	inputShortcutName
	inputShortcutPrompt
	inputShortcutDeleteConfirm
```

Change it to:

```go
	inputShortcutSelect
	inputShortcutName
	inputShortcutDescription
	inputShortcutPrompt
	inputShortcutDeleteConfirm
```

- [ ] **Step 2: Add the description input and temp value to the `model` struct**

In `model.go`, locate the AI shortcuts fields block (around lines 125-134) and add two lines so it reads:

```go
	// AI shortcuts
	shortcuts              []AIShortcut
	shortcutCursor         int
	shortcutEditing        int
	shortcutTempName       string
	shortcutTempDescription string
	shortcutOnSelection    bool
	shortcutPending        bool // true when shortcut awaits provider config completion
	shortcutNameInput      textinput.Model
	shortcutDescriptionInput textinput.Model
	shortcutPromptInput    textinput.Model
	activeShortcutProvider string // which AI provider runs shortcuts; cycled with 'p'
```

- [ ] **Step 3: Initialize the new text input in the model constructor**

In `model.go`, find the block that creates `sn` and `sp` (around lines 169-175). Add an `sd` between them so the block reads:

```go
	sn := textinput.New()
	sn.Placeholder = "shortcut name"
	sn.CharLimit = 256

	sd := textinput.New()
	sd.Placeholder = "short description"
	sd.CharLimit = 120

	sp := textinput.New()
	sp.Placeholder = "prompt template"
	sp.CharLimit = 500
```

Then, in the `m := model{...}` struct literal (around lines 181-198), add the new field assignment next to the other shortcut inputs. Replace:

```go
		shortcutNameInput:      sn,
		shortcutPromptInput:    sp,
```

with:

```go
		shortcutNameInput:        sn,
		shortcutDescriptionInput: sd,
		shortcutPromptInput:      sp,
```

(Adjust spacing of the surrounding fields only if needed to keep `gofmt` happy; Go's formatter handles alignment automatically.)

- [ ] **Step 4: Route the new input mode to a handler**

In `model.go`, find the Update dispatch block (around lines 649-656):

```go
	case inputShortcutSelect:
		return m.handleShortcutSelect(msg)
	case inputShortcutName:
		return m.handleShortcutName(msg)
	case inputShortcutPrompt:
		return m.handleShortcutPrompt(msg)
	case inputShortcutDeleteConfirm:
		return m.handleShortcutDeleteConfirm(msg)
```

Change it to:

```go
	case inputShortcutSelect:
		return m.handleShortcutSelect(msg)
	case inputShortcutName:
		return m.handleShortcutName(msg)
	case inputShortcutDescription:
		return m.handleShortcutDescription(msg)
	case inputShortcutPrompt:
		return m.handleShortcutPrompt(msg)
	case inputShortcutDeleteConfirm:
		return m.handleShortcutDeleteConfirm(msg)
```

- [ ] **Step 5: Add a status-bar case for the description step**

In `model.go`, find the status-bar chain (around lines 1360-1365):

```go
	} else if m.inputMode == inputShortcutName {
		statusView = statusBarStyle.Width(m.width).Render(
			"Shortcut name: " + m.shortcutNameInput.View())
	} else if m.inputMode == inputShortcutPrompt {
		statusView = statusBarStyle.Width(m.width).Render(
			"Prompt: " + m.shortcutPromptInput.View())
```

Change it to:

```go
	} else if m.inputMode == inputShortcutName {
		statusView = statusBarStyle.Width(m.width).Render(
			"Shortcut name: " + m.shortcutNameInput.View())
	} else if m.inputMode == inputShortcutDescription {
		statusView = statusBarStyle.Width(m.width).Render(
			"Description: " + m.shortcutDescriptionInput.View())
	} else if m.inputMode == inputShortcutPrompt {
		statusView = statusBarStyle.Width(m.width).Render(
			"Prompt: " + m.shortcutPromptInput.View())
```

- [ ] **Step 6: Build to catch compile errors (handler does not exist yet)**

Run: `go build ./...`
Expected: FAIL — `undefined: (model).handleShortcutDescription`. This is expected; Task 5 adds the handler.

- [ ] **Step 7: Do not commit yet**

The repo will not build standalone at this point. We'll commit after Task 5 wires the handler in.

---

### Task 5: Implement `handleShortcutDescription` and wire flow

**Files:**
- Modify: `shortcuts_input.go`

- [ ] **Step 1: Rewrite `shortcuts_input.go` with the three-step flow**

Replace the full contents of `shortcuts_input.go` with:

```go
package main

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleShortcutSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.shortcutCursor > 0 {
			m.shortcutCursor--
		}
	case "down", "j":
		if m.shortcutCursor < len(m.shortcuts)-1 {
			m.shortcutCursor++
		}
	case "enter":
		if len(m.shortcuts) == 0 || m.shortcutCursor >= len(m.shortcuts) {
			return m, nil
		}
		shortcut := m.shortcuts[m.shortcutCursor]
		provider := m.activeShortcutProvider
		if provider == "" {
			provider = defaultAIShortcutProvider
		}
		plugin := pluginByName(m.plugins, provider)
		if plugin == nil {
			m.errMsg = "Unknown AI shortcut provider: " + provider
			return m, nil
		}
		cfg, err := loadPluginConfig(provider)
		if err != nil || !pluginConfigComplete(plugin.ConfigFields(), cfg) {
			m.shortcutPending = true
			m.pluginActive = plugin
			m.pluginConfigFields = plugin.ConfigFields()
			m.pluginConfigIndex = 0
			m.pluginConfigValues = make(map[string]string)
			m.inputMode = inputPluginConfig
			m.pluginConfigInput = newPluginConfigInput(m.pluginConfigFields[0])
			return m, textinput.Blink
		}
		content := m.editor.Value()
		m.shortcutOnSelection = m.editor.selActive
		if m.shortcutOnSelection {
			content = m.editor.SelectedText()
		}
		m.pluginDiffOriginal = content
		m.pluginProcessing = true
		m.inputMode = inputNone
		return m, runShortcutCmd(shortcut, content, provider, cfg)
	case "p":
		if len(m.plugins) <= 1 {
			return m, nil
		}
		allNames := make([]string, 0, len(m.plugins))
		for _, p := range m.plugins {
			allNames = append(allNames, p.Name())
		}
		next := cycleShortcutProvider(m.activeShortcutProvider, allNames)
		if next != m.activeShortcutProvider {
			m.activeShortcutProvider = next
			if cfg, err := loadConfig(); err == nil {
				cfg.AIShortcutProvider = next
				_ = saveConfig(cfg)
			}
		}
	case "e":
		if len(m.shortcuts) > 0 && m.shortcutCursor < len(m.shortcuts) {
			m.shortcutEditing = m.shortcutCursor
			m.inputMode = inputShortcutName
			m.shortcutNameInput.SetValue(m.shortcuts[m.shortcutCursor].Name)
			cmd := m.shortcutNameInput.Focus()
			return m, cmd
		}
	case "d":
		if len(m.shortcuts) > 0 && m.shortcutCursor < len(m.shortcuts) {
			m.inputMode = inputShortcutDeleteConfirm
		}
	case "esc":
		m.inputMode = inputNone
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

func (m model) handleShortcutName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := m.shortcutNameInput.Value()
		if name == "" {
			return m, nil
		}
		m.shortcutTempName = name
		m.inputMode = inputShortcutDescription
		if m.shortcutEditing >= 0 {
			m.shortcutDescriptionInput.SetValue(m.shortcuts[m.shortcutEditing].Description)
		} else {
			m.shortcutDescriptionInput.SetValue("")
		}
		cmd := m.shortcutDescriptionInput.Focus()
		return m, cmd
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutNameInput, cmd = m.shortcutNameInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutDescription(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		desc := m.shortcutDescriptionInput.Value()
		if desc == "" {
			return m, nil
		}
		m.shortcutTempDescription = desc
		m.inputMode = inputShortcutPrompt
		if m.shortcutEditing >= 0 {
			m.shortcutPromptInput.SetValue(m.shortcuts[m.shortcutEditing].Prompt)
		} else {
			m.shortcutPromptInput.SetValue("")
		}
		cmd := m.shortcutPromptInput.Focus()
		return m, cmd
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutDescriptionInput, cmd = m.shortcutDescriptionInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
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
	case "esc":
		m.inputMode = inputNone
		m.shortcutEditing = -1
	case "ctrl+q":
		if m.isDirty() {
			m.inputMode = inputUnsavedGuard
			m.pendingAction = pendingQuit
			return m, nil
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.shortcutPromptInput, cmd = m.shortcutPromptInput.Update(msg)
	return m, cmd
}

func (m model) handleShortcutDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		if m.shortcutCursor < len(m.shortcuts) {
			m.shortcuts = append(m.shortcuts[:m.shortcutCursor], m.shortcuts[m.shortcutCursor+1:]...)
			if err := saveShortcuts(m.shortcuts); err != nil {
				m.errMsg = "Failed to save shortcuts: " + err.Error()
			}
			if m.shortcutCursor >= len(m.shortcuts) && m.shortcutCursor > 0 {
				m.shortcutCursor--
			}
		}
		if len(m.shortcuts) == 0 {
			m.inputMode = inputNone
		} else {
			m.inputMode = inputShortcutSelect
		}
	case "n", "esc":
		m.inputMode = inputShortcutSelect
	}
	return m, nil
}
```

- [ ] **Step 2: Build and verify the binary compiles**

Run: `go build ./...`
Expected: success (no output).

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add model.go shortcuts_input.go
git commit -m "feat(shortcuts): add description step to create/edit flow"
```

---

### Task 6: Input-flow test for the new description step

**Files:**
- Create: `shortcuts_input_test.go`

- [ ] **Step 1: Write tests that drive the create and edit flows**

Create `shortcuts_input_test.go`:

```go
package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg returns a tea.KeyMsg for a single-character key. For special keys
// (like "enter", "esc") callers should construct the tea.KeyMsg directly.
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func pressEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func pressEsc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }

func newTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	vault := t.TempDir()
	return newModel(vault, nil, "")
}

func TestShortcutFlow_Create_GoesThroughDescriptionStep(t *testing.T) {
	m := newTestModel(t)
	m.shortcutEditing = -1
	m.inputMode = inputShortcutName
	m.shortcutNameInput.SetValue("mytest")

	next, _ := m.handleShortcutName(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputShortcutDescription {
		t.Fatalf("after name: inputMode = %v, want inputShortcutDescription", nm.inputMode)
	}
	if nm.shortcutTempName != "mytest" {
		t.Errorf("shortcutTempName = %q, want %q", nm.shortcutTempName, "mytest")
	}

	// Empty description: Enter should not advance.
	nm.shortcutDescriptionInput.SetValue("")
	next2, _ := nm.handleShortcutDescription(pressEnter())
	nm2 := next2.(model)
	if nm2.inputMode != inputShortcutDescription {
		t.Errorf("empty description should block advance, got %v", nm2.inputMode)
	}

	// Non-empty description: advances to prompt, captures temp value.
	nm2.shortcutDescriptionInput.SetValue("short desc")
	next3, _ := nm2.handleShortcutDescription(pressEnter())
	nm3 := next3.(model)
	if nm3.inputMode != inputShortcutPrompt {
		t.Fatalf("after description: inputMode = %v, want inputShortcutPrompt", nm3.inputMode)
	}
	if nm3.shortcutTempDescription != "short desc" {
		t.Errorf("shortcutTempDescription = %q, want %q", nm3.shortcutTempDescription, "short desc")
	}

	// Prompt saves the full shortcut including description.
	nm3.shortcutPromptInput.SetValue("do the thing")
	next4, _ := nm3.handleShortcutPrompt(pressEnter())
	nm4 := next4.(model)
	if nm4.inputMode != inputNone {
		t.Errorf("after prompt: inputMode = %v, want inputNone", nm4.inputMode)
	}
	found := false
	for _, s := range nm4.shortcuts {
		if s.Name == "mytest" {
			found = true
			if s.Description != "short desc" {
				t.Errorf("saved description = %q, want %q", s.Description, "short desc")
			}
			if s.Prompt != "do the thing" {
				t.Errorf("saved prompt = %q, want %q", s.Prompt, "do the thing")
			}
		}
	}
	if !found {
		t.Error("new shortcut 'mytest' not saved")
	}
}

func TestShortcutFlow_Edit_PrefillsDescription(t *testing.T) {
	m := newTestModel(t)
	m.shortcuts = []AIShortcut{
		{Name: "n1", Description: "d1", Prompt: "p1"},
	}
	m.shortcutCursor = 0
	m.shortcutEditing = 0
	m.inputMode = inputShortcutName
	m.shortcutNameInput.SetValue("n1")

	next, _ := m.handleShortcutName(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputShortcutDescription {
		t.Fatalf("inputMode = %v, want inputShortcutDescription", nm.inputMode)
	}
	if got := nm.shortcutDescriptionInput.Value(); got != "d1" {
		t.Errorf("description input not prefilled: got %q, want %q", got, "d1")
	}
}

func TestShortcutFlow_Description_EscCancels(t *testing.T) {
	m := newTestModel(t)
	m.shortcutEditing = 3
	m.inputMode = inputShortcutDescription
	m.shortcutDescriptionInput.SetValue("typed something")

	next, _ := m.handleShortcutDescription(pressEsc())
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("after esc: inputMode = %v, want inputNone", nm.inputMode)
	}
	if nm.shortcutEditing != -1 {
		t.Errorf("after esc: shortcutEditing = %d, want -1", nm.shortcutEditing)
	}
}
```

- [ ] **Step 2: Run the new tests**

Run: `go test ./... -run TestShortcutFlow -v`
Expected: PASS for all three tests.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add shortcuts_input_test.go
git commit -m "test(shortcuts): cover description step in create/edit flow"
```

---

### Task 7: End-to-end manual verification

**Files:** none

- [ ] **Step 1: Build the binary**

Run: `go build -o /tmp/clipad-desc ./...`
Expected: success.

- [ ] **Step 2: Run the binary against a scratch vault**

Run:

```bash
mkdir -p /tmp/clipad-desc-vault
XDG_CONFIG_HOME=/tmp/clipad-desc-xdg /tmp/clipad-desc /tmp/clipad-desc-vault
```

- [ ] **Step 3: Verify the modal**

Press Ctrl+G. Expected:
- Each of the 23 seeded shortcuts shows as `name   — description` with names aligned.
- Descriptions render in a dimmer foreground than the names.
- Navigating with j/k highlights the current row; its description stays visible.

- [ ] **Step 4: Verify the create flow**

Press Ctrl+L (or whatever create binding is — based on the empty-state hint "Press Ctrl+L to create one"). Expected status-bar prompts:
1. `Shortcut name:` — type `demo`, press Enter.
2. `Description:` — type `demo description`, press Enter.
3. `Prompt:` — type `transform text`, press Enter.

Re-open Ctrl+G and confirm the new row appears with the description.

- [ ] **Step 5: Verify the edit flow**

Navigate to `demo`, press `e`. Verify:
1. Name prefilled with `demo`.
2. After Enter, description prefilled with `demo description`.
3. After Enter, prompt prefilled with `transform text`.

Change the description to something else; after Enter + Enter, the modal shows the new description.

- [ ] **Step 6: Verify Esc cancels from the description step**

Create another shortcut; at the description step, press Esc. Expected: modal closes without the partial shortcut being saved.

- [ ] **Step 7: Clean up**

```bash
rm -rf /tmp/clipad-desc /tmp/clipad-desc-vault /tmp/clipad-desc-xdg
```

- [ ] **Step 8: Run the full test suite one last time**

Run: `go test ./...`
Expected: PASS.

No commit needed — this task is verification only.

---

## Summary

After Task 7, the branch contains six commits:
1. Add `Description` field to `AIShortcut`.
2. Seed descriptions for the 23 default shortcuts.
3. Two-column selector rendering with dim descriptions.
4. (Part of commit 5) — model plumbing combined with handler wiring.
5. Description step in create/edit flow (commit combines Tasks 4+5).
6. Input-flow tests for the description step.

The final state satisfies every section of `docs/superpowers/specs/2026-04-21-shortcut-descriptions-design.md`: schema change, required description step, two-column rendering with truncation and alignment, seeded defaults, and tests across data, rendering, and input-flow layers.
