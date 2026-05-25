# Daily Notes + Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a lightweight template engine plus two keystrokes — `Alt+D` (open/create today's daily note) and `Alt+T` (new note from a picked template).

**Architecture:** A pure, regex-based template renderer lives in `templates.go` with an embedded default `daily.md` seeded lazily into `~/.config/clipad/templates`. Two `*model` flow methods drive the keystrokes; `Alt+T` adds a picker modal + filename prompt reusing the existing `inputMode` / handler / status-bar patterns. All logic is unit-tested with deterministic injected clocks.

**Tech Stack:** Go, Bubble Tea (`charmbracelet/bubbletea`, `bubbles/textinput`), lipgloss. Single `package main` in the repo root; tests are `*_test.go` in the same package.

**Spec:** `docs/superpowers/specs/2026-05-25-daily-notes-templates-design.md`

---

## File Structure

- **Create `templates.go`** — engine + flow methods: `templatesDir()`, `seedDefaultTemplate()`, `listTemplates()`, `renderTemplate()`, the embedded default, and `*model` methods `openDailyNote()`, `startTemplatePicker()`, `targetDirFromSelection()`.
- **Create `defaults/daily.md`** — embedded default daily template.
- **Create `templates_input.go`** — `handleTemplatePick()`, `handleTemplateName()`.
- **Create `templates_modal.go`** — `templatePickerView()` (reuses styles from `shortcuts_modal.go`).
- **Create `templates_test.go`** — engine + flow tests.
- **Modify `model.go`** — `inputMode` constants, `model` struct fields, `newModel` init, `Alt+D`/`Alt+T` key cases, `handleInputMode` routing, `View()` rendering.
- **Modify `help_modal.go`** — two Global help entries.

Note: `shortcuts.go` already owns the unrelated *AI* shortcuts; the keyboard dispatch the task refers to actually lives in `model.go`'s `Update`. We extend `model.go`, not `shortcuts.go`.

---

### Task 1: Template renderer (`renderTemplate`)

**Files:**
- Create: `templates.go`
- Test: `templates_test.go`

- [ ] **Step 1: Write the failing tests**

Create `templates_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestRenderTemplate_Variables(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	in := "d={{date}} t={{time}} y={{yesterday}} v={{vault}} c={{date:02 Jan 2006}}"
	want := "d=2026-05-25 t=14:30 y=2026-05-24 v=/tmp/vault c=25 May 2026"
	got := renderTemplate(in, now, "/tmp/vault")
	if got != want {
		t.Errorf("renderTemplate:\n got  %q\n want %q", got, want)
	}
}

func TestRenderTemplate_UnknownPlaceholdersUntouched(t *testing.T) {
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	in := "{{foo}} {{date}} {{bar:x}} literal {{ }}"
	want := "{{foo}} 2026-05-25 {{bar:x}} literal {{ }}"
	got := renderTemplate(in, now, "/v")
	if got != want {
		t.Errorf("renderTemplate:\n got  %q\n want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestRenderTemplate -v`
Expected: FAIL — `undefined: renderTemplate`.

- [ ] **Step 3: Implement `renderTemplate`**

Create `templates.go`:

```go
package main

import (
	"regexp"
	"time"
)

// templateVarRe matches {{name}} and {{name:layout}} for the supported
// variable names only. Unknown placeholders never match and pass through
// untouched. The layout cannot contain braces (Go reference layouts never do).
var templateVarRe = regexp.MustCompile(`\{\{(date|time|yesterday|vault)(?::([^{}]*))?\}\}`)

// renderTemplate substitutes template variables in content. now is injected so
// rendering is deterministic and unit-testable.
func renderTemplate(content string, now time.Time, vault string) string {
	return templateVarRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := templateVarRe.FindStringSubmatch(match)
		name, layout := sub[1], sub[2]
		switch name {
		case "date":
			if layout != "" {
				return now.Format(layout)
			}
			return now.Format("2006-01-02")
		case "time":
			return now.Format("15:04")
		case "yesterday":
			return now.AddDate(0, 0, -1).Format("2006-01-02")
		case "vault":
			return vault
		}
		return match
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run TestRenderTemplate -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add templates.go templates_test.go
git commit -m "feat(templates): regex-based template variable renderer"
```

---

### Task 2: Seed default template (`templatesDir`, `seedDefaultTemplate`)

**Files:**
- Create: `defaults/daily.md`
- Modify: `templates.go`
- Test: `templates_test.go`

- [ ] **Step 1: Create the embedded default template**

Create `defaults/daily.md` with exactly this content:

```
# {{date:Monday, 2 January 2006}}

## Notes

## Tasks
- [ ] 

---
Yesterday: [[{{yesterday}}]]
```

- [ ] **Step 2: Write the failing tests**

Append to `templates_test.go`:

```go
import (
	"os"
	"path/filepath"
	"strings"
)
```

(Merge these into the existing `import (...)` block; `os`, `path/filepath`, and `strings` are additions alongside `testing` and `time`.)

```go
func TestSeedDefaultTemplate_CreatesWhenAbsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := seedDefaultTemplate(); err != nil {
		t.Fatalf("seedDefaultTemplate: %v", err)
	}
	path := filepath.Join(templatesDir(), "daily.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("daily.md not created: %v", err)
	}
	if !strings.Contains(string(data), "{{date") {
		t.Errorf("seeded template missing date variable:\n%s", data)
	}
}

func TestSeedDefaultTemplate_DoesNotOverwrite(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(templatesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(templatesDir(), "daily.md")
	if err := os.WriteFile(path, []byte("custom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := seedDefaultTemplate(); err != nil {
		t.Fatalf("seedDefaultTemplate: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "custom" {
		t.Errorf("seed overwrote existing template: got %q", data)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... -run TestSeedDefaultTemplate -v`
Expected: FAIL — `undefined: seedDefaultTemplate` / `undefined: templatesDir`.

- [ ] **Step 4: Implement `templatesDir` and `seedDefaultTemplate`**

Edit `templates.go` — update the imports and add the functions. New import block:

```go
import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)
```

Add after the imports:

```go
//go:embed defaults/daily.md
var defaultDailyTemplate []byte

// templatesDir is ~/.config/clipad/templates, honoring XDG_CONFIG_HOME,
// mirroring shortcutsPath().
func templatesDir() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "clipad", "templates")
}

// seedDefaultTemplate writes the embedded daily.md into templatesDir only if it
// is absent. Idempotent; never overwrites a user-edited template.
func seedDefaultTemplate() error {
	dir := templatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating templates dir: %w", err)
	}
	path := filepath.Join(dir, "daily.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking daily template: %w", err)
	}
	if err := os.WriteFile(path, defaultDailyTemplate, 0o644); err != nil {
		return fmt.Errorf("seeding daily template: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./... -run TestSeedDefaultTemplate -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add templates.go templates_test.go defaults/daily.md
git commit -m "feat(templates): seed embedded default daily.md template"
```

---

### Task 3: List templates (`listTemplates`)

**Files:**
- Modify: `templates.go`
- Test: `templates_test.go`

- [ ] **Step 1: Write the failing test**

Append to `templates_test.go`:

```go
func TestListTemplates_SortsAndFiltersMd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := templatesDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zzz.md", "daily.md", "note.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := listTemplates()
	if err != nil {
		t.Fatalf("listTemplates: %v", err)
	}
	want := []string{"daily.md", "zzz.md"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("listTemplates = %v, want %v", got, want)
	}
}

func TestListTemplates_MissingDirReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := listTemplates()
	if err != nil {
		t.Fatalf("listTemplates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listTemplates on missing dir = %v, want empty", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestListTemplates -v`
Expected: FAIL — `undefined: listTemplates`.

- [ ] **Step 3: Implement `listTemplates`**

Add `"sort"` and `"strings"` to the `templates.go` import block (final block):

```go
import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)
```

Add the function:

```go
// listTemplates returns the sorted *.md basenames in templatesDir. A missing
// directory yields an empty slice, not an error.
func listTemplates() ([]string, error) {
	entries, err := os.ReadDir(templatesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading templates dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestListTemplates -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add templates.go templates_test.go
git commit -m "feat(templates): list *.md templates sorted"
```

---

### Task 4: Daily note flow (`Alt+D`)

**Files:**
- Modify: `templates.go` (add `openDailyNote`, `targetDirFromSelection`)
- Modify: `model.go:843` (add `case "alt+d"` after the `ctrl+o` case, before `case "tab"`)
- Modify: `help_modal.go:53` (add Global help entry)
- Test: `templates_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `templates_test.go`:

```go
func TestOpenDailyNote_CreatesFromTemplate(t *testing.T) {
	m := newTestModel(t)
	m.openDailyNote()
	if m.errMsg != "" {
		t.Fatalf("unexpected errMsg: %s", m.errMsg)
	}
	today := time.Now().Format("2006-01-02")
	want := filepath.Join(m.vault, "daily", today+".md")
	if m.currentFile != want {
		t.Errorf("currentFile = %q, want %q", m.currentFile, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("daily note not created: %v", err)
	}
	body := m.editor.Value()
	if strings.Contains(body, "{{date") || strings.Contains(body, "{{yesterday") {
		t.Errorf("template not rendered:\n%s", body)
	}
	if !strings.Contains(body, today[:4]) { // year present somewhere
		t.Errorf("rendered body missing date:\n%s", body)
	}
}

func TestOpenDailyNote_OpensExistingWithoutRerender(t *testing.T) {
	m := newTestModel(t)
	today := time.Now().Format("2006-01-02")
	dir := filepath.Join(m.vault, "daily")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, today+".md")
	if err := os.WriteFile(path, []byte("EXISTING {{date}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.openDailyNote()
	if m.editor.Value() != "EXISTING {{date}}" {
		t.Errorf("existing note was modified: %q", m.editor.Value())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... -run TestOpenDailyNote -v`
Expected: FAIL — `m.openDailyNote undefined`.

- [ ] **Step 3: Implement the flow methods**

Append to `templates.go`:

```go
// targetDirFromSelection returns the directory a new note should land in based
// on the current tree selection: the selected folder, the parent of a selected
// file, or the vault root.
func (m *model) targetDirFromSelection() string {
	dir := m.vault
	if node := m.tree.selectedNode(); node != nil {
		if node.IsDir {
			dir = node.Path
		} else {
			dir = filepath.Dir(node.Path)
		}
	}
	return dir
}

// openDailyNote opens <vault>/daily/YYYY-MM-DD.md, creating it from the daily
// template when absent. An existing note is opened as-is (not re-rendered).
func (m *model) openDailyNote() {
	if err := seedDefaultTemplate(); err != nil {
		m.errMsg = "Template setup failed: " + err.Error()
		return
	}
	now := time.Now()
	dailyDir := filepath.Join(m.vault, "daily")
	path := filepath.Join(dailyDir, now.Format("2006-01-02")+".md")

	if _, err := os.Stat(path); err == nil {
		m.openFile(path)
		m.activePanel = editorPanel
		m.editor.Focus()
		return
	}

	tmpl, err := os.ReadFile(filepath.Join(templatesDir(), "daily.md"))
	if err != nil {
		m.errMsg = "Read template failed: " + err.Error()
		return
	}
	if err := os.MkdirAll(dailyDir, 0o755); err != nil {
		m.errMsg = fmt.Sprintf("Create dir failed: %v", err)
		return
	}
	rendered := renderTemplate(string(tmpl), now, m.vault)
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		m.errMsg = fmt.Sprintf("Save failed: %v", err)
		return
	}
	m.openFile(path)
	m.activePanel = editorPanel
	m.editor.Focus()
	m.refreshTree()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run TestOpenDailyNote -v`
Expected: PASS (both tests).

- [ ] **Step 5: Wire the `Alt+D` keybinding**

In `model.go`, immediately after the `ctrl+o` case block (ends at line 843 with `}`) and before `case "tab":` (line 845), insert:

```go
		case "alt+d":
			if m.vault == "" {
				m.errMsg = "no vault configured"
				return m, nil
			}
			m.openDailyNote()
			return m, nil
```

- [ ] **Step 6: Add the help entry**

In `help_modal.go`, in the `Global` section, after the `{"Ctrl+K", "Ask your vault (chat)"},` line (line 53) insert:

```go
			{"Alt+D", "Open today's daily note"},
```

- [ ] **Step 7: Write an Update-level dispatch test**

Append to `templates_test.go`:

```go
import tea "github.com/charmbracelet/bubbletea"
```

(Merge into the existing import block.)

```go
func TestUpdate_AltD_OpensDailyNote(t *testing.T) {
	m := newTestModel(t)
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true}
	next, _ := m.Update(key)
	nm := next.(model)
	today := time.Now().Format("2006-01-02")
	want := filepath.Join(nm.vault, "daily", today+".md")
	if nm.currentFile != want {
		t.Errorf("after Alt+D currentFile = %q, want %q", nm.currentFile, want)
	}
}
```

- [ ] **Step 8: Run the full test build to verify**

Run: `go test ./... -run 'TestUpdate_AltD|TestOpenDailyNote' -v`
Expected: PASS. Also run `go build ./...` — expect no output (success).

- [ ] **Step 9: Commit**

```bash
git add templates.go model.go help_modal.go templates_test.go
git commit -m "feat(templates): Alt+D opens/creates today's daily note"
```

---

### Task 5: New-from-template flow (`Alt+T`)

**Files:**
- Modify: `model.go` (inputMode constants `inputTemplatePick`/`inputTemplateName`; struct fields; `newModel` init; `handleInputMode` routing; `Alt+T` key case; `View()` picker + status-bar rendering)
- Create: `templates_input.go` (`handleTemplatePick`, `handleTemplateName`)
- Create: `templates_modal.go` (`templatePickerView`)
- Modify: `help_modal.go` (Global entry)
- Modify: `templates.go` (add `startTemplatePicker`)
- Test: `templates_test.go`

- [ ] **Step 1: Add the inputMode constants**

In `model.go`, inside the `const (...)` block ending at line 76, add before the closing `)` (after `inputDelegateName`):

```go
	inputTemplatePick
	inputTemplateName
```

- [ ] **Step 2: Add model struct fields**

In `model.go`, inside `type model struct`, after the startup fields (after line 203 `startupDone bool`) and before the closing `}`, add:

```go
	// Templates (Alt+T new-from-template)
	templateList      []string
	templateCursor    int
	templateChosen    string // basename of the picked template
	templateTargetDir string // directory the new note lands in
	templateNameInput textinput.Model
```

- [ ] **Step 3: Initialize the filename input in `newModel`**

In `model.go` `newModel`, after the `del` textinput setup (after line 265 `del.Prompt = "Move to: "`), add:

```go
	tn := textinput.New()
	tn.Placeholder = "filename (no .md needed)"
	tn.CharLimit = 200
```

And in the `model{...}` struct literal, after `delegateInput: del,` (line 288), add:

```go
		templateNameInput:        tn,
```

- [ ] **Step 4: Write the failing handler tests**

Append to `templates_test.go`:

```go
func seedTemplate(t *testing.T, name, body string) {
	t.Helper()
	dir := templatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTemplatePick_EnterAdvancesToName(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputTemplatePick
	m.templateList = []string{"daily.md", "meeting.md"}
	m.templateCursor = 1
	next, _ := m.handleTemplatePick(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputTemplateName {
		t.Errorf("inputMode = %v, want inputTemplateName", nm.inputMode)
	}
	if nm.templateChosen != "meeting.md" {
		t.Errorf("templateChosen = %q, want meeting.md", nm.templateChosen)
	}
}

func TestTemplatePick_EscCancels(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = inputTemplatePick
	m.templateList = []string{"daily.md"}
	next, _ := m.handleTemplatePick(pressEsc())
	if next.(model).inputMode != inputNone {
		t.Errorf("Esc did not cancel picker")
	}
}

func TestTemplateName_CreatesRenderedFileAndOpens(t *testing.T) {
	m := newTestModel(t)
	seedTemplate(t, "note.md", "Hello {{date}}")
	m.inputMode = inputTemplateName
	m.templateChosen = "note.md"
	m.templateTargetDir = m.vault
	m.templateNameInput.SetValue("myidea")
	next, _ := m.handleTemplateName(pressEnter())
	nm := next.(model)
	if nm.inputMode != inputNone {
		t.Errorf("inputMode = %v, want inputNone", nm.inputMode)
	}
	path := filepath.Join(m.vault, "myidea.md")
	if nm.currentFile != path {
		t.Errorf("currentFile = %q, want %q", nm.currentFile, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if string(data) != "Hello "+today {
		t.Errorf("rendered content = %q, want %q", data, "Hello "+today)
	}
}

func TestTemplateName_RejectsExistingFile(t *testing.T) {
	m := newTestModel(t)
	seedTemplate(t, "note.md", "x")
	existing := filepath.Join(m.vault, "dupe.md")
	if err := os.WriteFile(existing, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.inputMode = inputTemplateName
	m.templateChosen = "note.md"
	m.templateTargetDir = m.vault
	m.templateNameInput.SetValue("dupe.md")
	next, _ := m.handleTemplateName(pressEnter())
	nm := next.(model)
	if nm.errMsg == "" {
		t.Errorf("expected errMsg for existing file")
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "ORIGINAL" {
		t.Errorf("existing file overwritten: %q", data)
	}
}

func TestUpdate_AltT_OpensPicker(t *testing.T) {
	m := newTestModel(t)
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}, Alt: true}
	next, _ := m.Update(key)
	nm := next.(model)
	if nm.inputMode != inputTemplatePick {
		t.Errorf("after Alt+T inputMode = %v, want inputTemplatePick", nm.inputMode)
	}
	if len(nm.templateList) == 0 {
		t.Errorf("templateList empty; expected seeded daily.md")
	}
}
```

- [ ] **Step 5: Run the tests to verify they fail**

Run: `go test ./... -run 'TestTemplatePick|TestTemplateName|TestUpdate_AltT' -v`
Expected: FAIL — `m.handleTemplatePick undefined`, `m.handleTemplateName undefined`, `undefined: inputTemplatePick` (if Step 1 not yet compiled) etc.

- [ ] **Step 6: Implement `startTemplatePicker`**

Append to `templates.go`:

```go
// startTemplatePicker seeds the default, loads the template list, captures the
// target directory from the current selection, and opens the picker modal.
func (m *model) startTemplatePicker() {
	if err := seedDefaultTemplate(); err != nil {
		m.errMsg = "Template setup failed: " + err.Error()
		return
	}
	names, err := listTemplates()
	if err != nil {
		m.errMsg = "Listing templates failed: " + err.Error()
		return
	}
	if len(names) == 0 {
		m.errMsg = "No templates found"
		return
	}
	m.templateTargetDir = m.targetDirFromSelection()
	m.templateList = names
	m.templateCursor = 0
	m.templateChosen = ""
	m.inputMode = inputTemplatePick
}
```

- [ ] **Step 7: Implement the handlers**

Create `templates_input.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleTemplatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.templateCursor > 0 {
			m.templateCursor--
		}
	case "down", "j":
		if m.templateCursor < len(m.templateList)-1 {
			m.templateCursor++
		}
	case "enter":
		if len(m.templateList) == 0 || m.templateCursor >= len(m.templateList) {
			m.inputMode = inputNone
			return m, nil
		}
		m.templateChosen = m.templateList[m.templateCursor]
		m.inputMode = inputTemplateName
		m.templateNameInput.SetValue("")
		cmd := m.templateNameInput.Focus()
		return m, cmd
	case "esc":
		m.inputMode = inputNone
	}
	return m, nil
}

func (m model) handleTemplateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.templateNameInput.Value())
		if name == "" {
			return m, nil
		}
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		dir := m.templateTargetDir
		if dir == "" {
			dir = m.vault
		}
		fullPath := filepath.Join(dir, name)
		if _, err := os.Stat(fullPath); err == nil {
			m.errMsg = fmt.Sprintf("File already exists: %s", name)
			return m, nil
		}
		tmpl, err := os.ReadFile(filepath.Join(templatesDir(), m.templateChosen))
		if err != nil {
			m.errMsg = "Read template failed: " + err.Error()
			m.inputMode = inputNone
			return m, nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.errMsg = fmt.Sprintf("Create dir failed: %v", err)
			m.inputMode = inputNone
			return m, nil
		}
		rendered := renderTemplate(string(tmpl), time.Now(), m.vault)
		if err := os.WriteFile(fullPath, []byte(rendered), 0o644); err != nil {
			m.errMsg = fmt.Sprintf("Save failed: %v", err)
			m.inputMode = inputNone
			return m, nil
		}
		m.inputMode = inputNone
		m.openFile(fullPath)
		m.activePanel = editorPanel
		m.editor.Focus()
		m.refreshTree()
		return m, nil
	case "esc":
		m.inputMode = inputNone
		return m, nil
	}
	var cmd tea.Cmd
	m.templateNameInput, cmd = m.templateNameInput.Update(msg)
	return m, cmd
}
```

- [ ] **Step 8: Route the new input modes**

In `model.go` `handleInputMode`, after the `inputDelegateName` case (line 1069-1070) and before the closing `}` of the switch (line 1071), add:

```go
	case inputTemplatePick:
		return m.handleTemplatePick(msg)
	case inputTemplateName:
		return m.handleTemplateName(msg)
```

- [ ] **Step 9: Wire the `Alt+T` keybinding**

In `model.go`, immediately after the `case "alt+d":` block added in Task 4 (before `case "tab":`), insert:

```go
		case "alt+t":
			if m.vault == "" {
				m.errMsg = "no vault configured"
				return m, nil
			}
			m.startTemplatePicker()
			return m, nil
```

- [ ] **Step 10: Implement the picker view**

Create `templates_modal.go`:

```go
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// templatePickerView renders the template chooser, reusing the shortcut-picker
// styles defined in shortcuts_modal.go.
func templatePickerView(templates []string, cursor, width, height int) string {
	var rows []string
	for i, name := range templates {
		if i == cursor {
			rows = append(rows, shortcutCursorStyle.Render("> "+name))
		} else {
			rows = append(rows, shortcutItemStyle.Render("  "+name))
		}
	}
	hint := shortcutHintStyle.Render("Enter:select  ↑/↓:move  Esc:cancel")
	content := strings.Join(rows, "\n") + "\n" + hint
	return lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Render(content)
}
```

- [ ] **Step 11: Render the picker in `View()`**

In `model.go` `View()`, in the `rightView` if/else chain, after the `inputShortcutType` branch (ends line 1955) and before the `inputPluginDiff` branch (line 1956), insert:

```go
	} else if m.inputMode == inputTemplatePick {
		rightView = templatePickerView(m.templateList, m.templateCursor, m.editorWidth, m.editorHeight)
```

- [ ] **Step 12: Render the filename prompt in `View()`**

In `model.go` `View()`, in the `statusView` if/else chain, after the `inputDelegateName` branch (ends line 2083) and before the `inputReplaceSearch` branch (line 2084), insert:

```go
	} else if m.inputMode == inputTemplateName {
		statusView = statusBarStyle.Width(m.width).Render(
			"New from " + m.templateChosen + ": " + m.templateNameInput.View())
```

- [ ] **Step 13: Add the help entry**

In `help_modal.go` `Global` section, immediately after the `{"Alt+D", "Open today's daily note"},` line added in Task 4, insert:

```go
			{"Alt+T", "New note from template"},
```

- [ ] **Step 14: Run the tests to verify they pass**

Run: `go test ./... -run 'TestTemplatePick|TestTemplateName|TestUpdate_AltT' -v`
Expected: PASS (all five tests). Also run `go build ./...` — expect success.

- [ ] **Step 15: Commit**

```bash
git add templates.go templates_input.go templates_modal.go model.go help_modal.go templates_test.go
git commit -m "feat(templates): Alt+T new note from a picked template"
```

---

### Task 6: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Format and vet**

Run: `gofmt -l templates.go templates_input.go templates_modal.go templates_test.go model.go help_modal.go`
Expected: no output (all formatted). If any file is listed, run `gofmt -w <file>` and re-check.

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 2: Run the entire test suite**

Run: `go test ./...`
Expected: `ok  	clipad` (all packages pass). No failures.

- [ ] **Step 3: Build the binary**

Run: `go build -o /tmp/clipad-check ./... && echo BUILD_OK`
Expected: `BUILD_OK`.

- [ ] **Step 4: Manual smoke test (optional, requires a configured vault)**

Run `./clipad`, then:
- Press `Alt+D` → today's daily note opens in the editor, pre-filled from the template (heading shows today's date, "Yesterday" link shows yesterday). Press `Alt+D` again → reopens the same note unchanged.
- Press `Alt+T` → template picker lists `daily.md`; pick it, type a filename, press Enter → a new rendered note is created in the selected tree folder and opens.
- Press `Ctrl+?` → help shows `Alt+D` and `Alt+T` in the Global section.

- [ ] **Step 5: Commit any formatting fixes**

If Step 1 modified files:

```bash
git add -A
git commit -m "style(templates): gofmt"
```

Otherwise skip.

---

## Self-Review

**Spec coverage:**
- `Alt+D` daily note create/open from template → Task 4. ✓
- Templates at `~/.config/clipad/templates/*.md` + lazy-seeded `daily.md` (no overwrite) → Tasks 2, 3. ✓
- Variables `{{date}}`, `{{date:format}}`, `{{time}}`, `{{yesterday}}`, `{{vault}}`, unknown passthrough → Task 1. ✓
- `Alt+T` list → pick → filename prompt → create in current tree folder → Task 5. ✓
- New `templates.go`, key wiring, config-dir integration → Tasks 1-5. ✓
- Tests for date formatting and variable substitution → Task 1 (+ flow tests Tasks 4-5). ✓
- Help discoverability → Tasks 4, 5. ✓

**Placeholder scan:** No TBD/TODO; every code step contains complete code. ✓

**Type consistency:** `renderTemplate(content string, now time.Time, vault string) string`, `seedDefaultTemplate() error`, `listTemplates() ([]string, error)`, `templatesDir() string`, `(*model).openDailyNote()`, `(*model).startTemplatePicker()`, `(*model).targetDirFromSelection() string`, `(model).handleTemplatePick`, `(model).handleTemplateName`, `templatePickerView(templates []string, cursor, width, height int) string`, fields `templateList/templateCursor/templateChosen/templateTargetDir/templateNameInput`, constants `inputTemplatePick/inputTemplateName` — names match across all tasks. ✓
