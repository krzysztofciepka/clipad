# Statusbar Writing Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show adaptive writing metrics (word/char count while editing, reading time while previewing, selection stats when text is selected) in the statusbar.

**Architecture:** A new `metrics.go` holds pure, UI-free counting helpers (fence stripping, word/char counts, reading-time). `statusbar.go` gains fields plus a token-selection method and a width-fitting layout helper that places the metrics between the hint segment and the position/filename segment, shedding tokens to fit. `model.View()` populates the new fields from editor state.

**Tech Stack:** Go 1.26.1, Bubble Tea / lipgloss TUI, table-driven `testing` tests (package `main`).

---

### Task 1: Pure counting helpers in `metrics.go`

**Files:**
- Create: `metrics.go`
- Test: `metrics_test.go`

- [ ] **Step 1: Write the failing tests**

Create `metrics_test.go`:

```go
package main

import "testing"

func TestComputeMetricsPlainProse(t *testing.T) {
	words, chars := computeMetrics("The quick brown fox")
	if words != 4 {
		t.Errorf("words = %d, want 4", words)
	}
	if chars != 19 {
		t.Errorf("chars = %d, want 19", chars)
	}
}

func TestComputeMetricsStripsCodeFence(t *testing.T) {
	text := "hello world\n```\ncode here ignored\nmore code\n```\nafter fence"
	words, _ := computeMetrics(text)
	if words != 4 { // "hello world" + "after fence"; fenced content excluded
		t.Errorf("words = %d, want 4", words)
	}
}

func TestComputeMetricsFenceOnly(t *testing.T) {
	words, _ := computeMetrics("```\ncode\n```")
	if words != 0 {
		t.Errorf("words = %d, want 0", words)
	}
}

func TestStripCodeFencesUnterminated(t *testing.T) {
	got := stripCodeFences("prose line\n```\nunclosed code\nstill code")
	if got != "prose line" {
		t.Errorf("stripCodeFences = %q, want %q", got, "prose line")
	}
}

func TestComputeMetricsMultiByte(t *testing.T) {
	words, chars := computeMetrics("café über señor")
	if words != 3 {
		t.Errorf("words = %d, want 3", words)
	}
	if chars != 15 { // rune count, not byte count
		t.Errorf("chars = %d, want 15", chars)
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t\n"} {
		words, _ := computeMetrics(in)
		if words != 0 {
			t.Errorf("computeMetrics(%q) words = %d, want 0", in, words)
		}
	}
}

func TestReadingMinutes(t *testing.T) {
	tests := []struct {
		words int
		want  int
	}{
		{0, 0}, {1, 1}, {220, 1}, {221, 2}, {440, 2}, {441, 3},
	}
	for _, tt := range tests {
		if got := readingMinutes(tt.words); got != tt.want {
			t.Errorf("readingMinutes(%d) = %d, want %d", tt.words, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestComputeMetrics|TestStripCodeFences|TestReadingMinutes' ./...`
Expected: FAIL — `undefined: computeMetrics`, `undefined: stripCodeFences`, `undefined: readingMinutes` (compile error).

- [ ] **Step 3: Write the implementation**

Create `metrics.go`:

```go
package main

import (
	"strings"
	"unicode/utf8"
)

const readingWPM = 220

// stripCodeFences removes fenced code blocks delimited by lines whose trimmed
// text begins with "```" (three or more backticks), including the fence lines
// themselves. An unterminated opening fence strips to the end of the text.
// Only backtick fences are handled; tilde (~~~) fences are out of scope.
func stripCodeFences(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// computeMetrics strips code fences, then counts words (unicode-whitespace
// split, empty tokens ignored) and chars (runes of the stripped text,
// including spaces and newlines).
func computeMetrics(text string) (words, chars int) {
	stripped := stripCodeFences(text)
	words = len(strings.Fields(stripped))
	chars = utf8.RuneCountInString(stripped)
	return words, chars
}

// readingMinutes returns ceil(words / readingWPM). Yields 0 for words == 0 and
// >= 1 for any non-empty prose, so the "floor at 1m for non-empty" rule is
// automatic.
func readingMinutes(words int) int {
	if words <= 0 {
		return 0
	}
	return (words + readingWPM - 1) / readingWPM
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestComputeMetrics|TestStripCodeFences|TestReadingMinutes' ./...`
Expected: PASS (ok clipad).

- [ ] **Step 5: Commit**

```bash
git add metrics.go metrics_test.go
git commit -m "feat(metrics): word/char counting and reading-time helpers"
```

---

### Task 2: Metric token selection and adaptive layout in `statusbar.go`

**Files:**
- Modify: `statusbar.go` (add fields to `StatusBar`, add `metricTokens` + `fitMetrics`, integrate into `View`)
- Test: `statusbar_test.go` (create)

- [ ] **Step 1: Write the failing tests**

Create `statusbar_test.go`:

```go
package main

import (
	"reflect"
	"testing"
)

func TestMetricTokensEditMode(t *testing.T) {
	sb := StatusBar{editorFocused: true, bufferText: "one two three"}
	prefix, tokens := sb.metricTokens()
	if prefix != "" {
		t.Errorf("prefix = %q, want \"\"", prefix)
	}
	want := []string{"3 words", "13 chars"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestMetricTokensPreviewMode(t *testing.T) {
	sb := StatusBar{editorFocused: true, previewMode: true, bufferText: "one two three"}
	_, tokens := sb.metricTokens()
	want := []string{"~1m read"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestMetricTokensSelection(t *testing.T) {
	sb := StatusBar{editorFocused: true, selectionActive: true, selectionText: "alpha beta"}
	prefix, tokens := sb.metricTokens()
	if prefix != "sel: " {
		t.Errorf("prefix = %q, want \"sel: \"", prefix)
	}
	want := []string{"2 words", "10 chars"}
	if !reflect.DeepEqual(tokens, want) {
		t.Errorf("tokens = %v, want %v", tokens, want)
	}
}

func TestMetricTokensHiddenWhenUnfocused(t *testing.T) {
	sb := StatusBar{editorFocused: false, bufferText: "lots of words here"}
	_, tokens := sb.metricTokens()
	if tokens != nil {
		t.Errorf("tokens = %v, want nil", tokens)
	}
}

func TestMetricTokensHiddenWhenEmpty(t *testing.T) {
	sb := StatusBar{editorFocused: true, bufferText: "   \n  "}
	_, tokens := sb.metricTokens()
	if tokens != nil {
		t.Errorf("tokens = %v, want nil", tokens)
	}
}

func TestFitMetricsDropsTokens(t *testing.T) {
	tokens := []string{"3 words", "13 chars"} // "3 words · 13 chars" == 18 wide
	if got := fitMetrics("", tokens, 100); got != "3 words · 13 chars" {
		t.Errorf("budget 100: got %q, want %q", got, "3 words · 13 chars")
	}
	if got := fitMetrics("", tokens, 10); got != "3 words" {
		t.Errorf("budget 10: got %q, want %q", got, "3 words")
	}
	if got := fitMetrics("", tokens, 3); got != "" {
		t.Errorf("budget 3: got %q, want \"\"", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestMetricTokens|TestFitMetrics' ./...`
Expected: FAIL — `sb.metricTokens undefined`, `undefined: fitMetrics`, and unknown fields `editorFocused`/`previewMode`/`selectionActive`/`bufferText`/`selectionText` (compile error).

- [ ] **Step 3: Add the new fields to the `StatusBar` struct**

In `statusbar.go`, extend the struct (after the existing `indexerStatus` field):

```go
type StatusBar struct {
	width         int
	treeActive    bool
	filename      string
	line          int
	col           int
	dirty         bool
	errMsg        string
	flashMsg      string // non-error flash message (e.g. "Auto-saved")
	fileOpen      bool
	indexerStatus string // e.g. "[idx 47/312]"

	// Writing metrics inputs (Task 29).
	editorFocused   bool   // activePanel == editorPanel
	previewMode     bool   // editorMode == modePreview
	selectionActive bool   // editor has an active selection
	bufferText      string // full editor buffer
	selectionText   string // selected text, "" when no selection
}
```

- [ ] **Step 4: Add `metricTokens` and `fitMetrics`**

In `statusbar.go`, add these two functions (e.g. just above `func (s StatusBar) View()`):

```go
// metricTokens returns the writing-metrics tokens to display (highest priority
// first) and an optional prefix, or ("", nil) when no metrics should show.
// Token order encodes the adaptive drop rule: drop the last token first, keep
// "W words" longest.
func (s StatusBar) metricTokens() (prefix string, tokens []string) {
	if !s.editorFocused {
		return "", nil
	}
	if s.selectionActive {
		words, chars := computeMetrics(s.selectionText)
		if words == 0 {
			return "", nil
		}
		return "sel: ", []string{
			fmt.Sprintf("%d words", words),
			fmt.Sprintf("%d chars", chars),
		}
	}
	words, chars := computeMetrics(s.bufferText)
	if words == 0 {
		return "", nil
	}
	if s.previewMode {
		return "", []string{fmt.Sprintf("~%dm read", readingMinutes(words))}
	}
	return "", []string{
		fmt.Sprintf("%d words", words),
		fmt.Sprintf("%d chars", chars),
	}
}

// fitMetrics joins tokens (highest priority first) with " · " after prefix,
// dropping the lowest-priority token until the result fits within budget.
// Returns "" if even the first token does not fit.
func fitMetrics(prefix string, tokens []string, budget int) string {
	for len(tokens) > 0 {
		s := prefix + strings.Join(tokens, " · ")
		if lipgloss.Width(s) <= budget {
			return s
		}
		tokens = tokens[:len(tokens)-1]
	}
	return ""
}
```

(`fmt`, `strings`, and `lipgloss` are already imported in `statusbar.go`.)

- [ ] **Step 5: Run the new tests to verify they pass**

Run: `go test -run 'TestMetricTokens|TestFitMetrics' ./...`
Expected: PASS.

- [ ] **Step 6: Integrate metrics into `View`'s layout**

In `statusbar.go`, locate this block in `View()`:

```go
	// Available content width (subtract padding)
	contentWidth := s.width - 2
	if contentWidth < 0 {
		contentWidth = 0
	}

	// Build left side, dropping hints from the end if they don't fit
	rightWidth := lipgloss.Width(right)
```

Insert the metrics block between the `contentWidth` clamp and `rightWidth`, so the result reads:

```go
	// Available content width (subtract padding)
	contentWidth := s.width - 2
	if contentWidth < 0 {
		contentWidth = 0
	}

	// Writing metrics, placed between the hints and the position/file segment.
	// Sheds tokens to fit and takes priority over trailing hints (which the
	// loop below drops to make room).
	if prefix, tokens := s.metricTokens(); len(tokens) > 0 {
		budget := contentWidth - lipgloss.Width(right) - 2
		if metrics := fitMetrics(prefix, tokens, budget); metrics != "" {
			if right == "" {
				right = metrics
			} else {
				right = metrics + "  " + right
			}
		}
	}

	// Build left side, dropping hints from the end if they don't fit
	rightWidth := lipgloss.Width(right)
```

- [ ] **Step 7: Run the full statusbar test file and build**

Run: `go test -run 'TestMetricTokens|TestFitMetrics' ./... && go build ./...`
Expected: PASS and a clean build (no output from `go build`).

- [ ] **Step 8: Commit**

```bash
git add statusbar.go statusbar_test.go
git commit -m "feat(statusbar): adaptive writing-metrics segment"
```

---

### Task 3: Wire editor state into the statusbar in `model.go`

**Files:**
- Modify: `model.go` (the `sb := StatusBar{...}` construction in `View()`, around line 2024)

- [ ] **Step 1: Populate the new fields**

In `model.go`, find the `StatusBar` construction in `View()`:

```go
	sb := StatusBar{
		width:         m.width,
		treeActive:    m.activePanel == treePanel,
		filename:      filename,
		line:          line + 1,
		col:           col + 1,
		dirty:         m.isDirty(),
		errMsg:        m.errMsg,
		fileOpen:      m.currentFile != "" || m.newNoteDir != "",
		indexerStatus: m.indexerStatus,
	}
```

Replace it with:

```go
	sb := StatusBar{
		width:           m.width,
		treeActive:      m.activePanel == treePanel,
		filename:        filename,
		line:            line + 1,
		col:             col + 1,
		dirty:           m.isDirty(),
		errMsg:          m.errMsg,
		fileOpen:        m.currentFile != "" || m.newNoteDir != "",
		indexerStatus:   m.indexerStatus,
		editorFocused:   m.activePanel == editorPanel,
		previewMode:     m.editorMode == modePreview,
		selectionActive: m.editor.selActive,
		bufferText:      m.editor.Value(),
	}
	if m.editor.selActive {
		sb.selectionText = m.editor.SelectedText()
	}
```

- [ ] **Step 2: Build and run the full test suite**

Run: `go build ./... && go test ./...`
Expected: clean build, all packages PASS.

- [ ] **Step 3: Manual smoke check (optional but recommended)**

Run `go run . <path-to-a-vault>` (or however the app is normally launched), open a note with prose and a fenced code block, and confirm:
- Editing shows `N words · C chars` to the left of the `line:col file` segment.
- Toggling preview (`^P`) shows `~Rm read`.
- Selecting text shows `sel: N words · C chars`.
- An empty note shows no metrics segment.
- Narrowing the terminal drops `chars` before `words`.

- [ ] **Step 4: Commit**

```bash
git add model.go
git commit -m "feat(statusbar): wire editor state into writing metrics"
```

---

## Notes for the implementer

- All counting logic is pure and lives in `metrics.go`; the statusbar only formats and lays out. Keep it that way.
- Char counts in tests are **rune** counts including spaces (e.g. `"one two three"` is 13). Reading-time uses 220 wpm with ceiling rounding.
- Do not add caching — buffers are small and recomputing per render is intentional (YAGNI).
