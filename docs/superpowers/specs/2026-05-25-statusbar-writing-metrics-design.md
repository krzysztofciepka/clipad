# Statusbar Writing Metrics — Design

**Date:** 2026-05-25
**Task:** Task 29 — word count + reading time in the statusbar

## Summary

Add lightweight writing metrics to the existing statusbar so the user can
eyeball note length and reading time at a glance. The displayed metric adapts
to the editor's submode and to whether a selection is active:

- **Edit mode:** `W words · C chars`
- **Preview mode:** `~Rm read` (reading time only)
- **Selection active (either mode):** `sel: N words · C chars`

All metrics are computed over the same code-fence-stripped text, so fenced code
blocks never inflate the word count or reading-time estimate.

## Behavior

- Metrics show only when the **editor panel is focused** (`activePanel ==
  editorPanel`). When the tree is focused, no metrics segment is rendered.
- **Empty notes are hidden entirely.** A note whose stripped text contains no
  words (empty, whitespace-only, or only fenced code) shows no metrics segment.
  The same applies to an empty selection.
- **Reading time** is `ceil(words / 220)` at 220 wpm. Because any non-empty
  prose has at least one word and `ceil(1/220) == 1`, the "floor at 1m for
  non-empty text" rule falls out automatically; empty prose yields 0 and is
  hidden.
- **Selection takes precedence** over the whole-note metric: when a selection is
  active and the editor is focused, the segment is `sel: N words · C chars`
  (no reading time), computed over the selected text.
- Recomputed **per render**. Buffers are small enough that caching is
  unnecessary (YAGNI).

## Counting rules

All three numbers are derived from a single canonical "prose text" produced by
stripping fenced code blocks:

- **Fence stripping:** a line whose trimmed text begins with ` ``` ` (three or
  more backticks) toggles fence state. The opening and closing fence lines and
  everything between them are removed. An unterminated fence (opening with no
  closing) strips to the end of the buffer. Only backtick fences are handled;
  tilde (`~~~`) fences are out of scope.
- **Words:** `len(strings.Fields(stripped))` — split on unicode whitespace,
  zero-length tokens ignored. Multi-byte-safe.
- **Chars:** `utf8.RuneCountInString(stripped)` — counts all runes including
  internal spaces and newlines. (A literal length metric; the spec's selection
  example `sel: 42 words · 280 chars` includes inter-word spaces.)

## Components

### `metrics.go` (new) — pure counting logic, no UI

```go
package main

// stripCodeFences removes fenced code blocks delimited by lines whose trimmed
// text begins with "```", including the fence lines themselves. An
// unterminated fence strips to end of text.
func stripCodeFences(text string) string

// computeMetrics strips code fences, then counts words (unicode-whitespace
// split, empty tokens ignored) and chars (runes of the stripped text).
func computeMetrics(text string) (words, chars int)

// readingMinutes returns ceil(words / 220). Yields 0 for words == 0 and >= 1
// for any non-empty prose.
func readingMinutes(words int) int
```

### `statusbar.go` — render + adaptive layout

New `StatusBar` fields:

```go
editorFocused   bool   // activePanel == editorPanel
previewMode     bool   // editorMode == modePreview
selectionActive bool   // editor.selActive
bufferText      string // editor.Value()
selectionText   string // editor.SelectedText(), "" when no selection
```

A helper builds the ordered token list (highest priority first) and an optional
prefix, or returns nothing (hidden):

```go
func (s StatusBar) metricTokens() (prefix string, tokens []string)
```

- not `editorFocused` → `("", nil)`
- `selectionActive`: `words, chars := computeMetrics(selectionText)`; if
  `words == 0` → hidden; else `prefix = "sel: "`, `tokens = ["N words", "C chars"]`
- else `words, chars := computeMetrics(bufferText)`; if `words == 0` → hidden
  - `previewMode` → `tokens = ["~Rm read"]` where `R = readingMinutes(words)`
  - else → `tokens = ["N words", "C chars"]`

Tokens are ordered so dropping from the end implements the spec's adaptive rule:
drop reading-time first (it is the only token in preview, so it simply
disappears), then chars, keeping `W words` longest.

### Layout

The metrics segment sits **between** the existing hint segment (left) and the
position/filename segment (right):

```
^S save  ^N new  ^P preview  …            137 words · 842 chars   12:5  notes/todo.md
```

Algorithm in `StatusBar.View()`:

1. Build the existing `right` segment (indexer / error / flash / `line:col
   filename`) and `left` hints exactly as today.
2. Compute the metrics string by joining tokens with ` · ` and prefix, fitting
   it into `budget = contentWidth - width(existingRight) - 2`; drop the
   lowest-priority token until it fits, or hide the segment if even the first
   token won't fit.
3. If non-empty, glue metrics to the left of the existing right segment
   (separated by two spaces): `right = metrics + "  " + existingRight` (skip the
   separator when `existingRight` is empty).
4. The existing hint-dropping loop then shrinks the hints to make room, so
   metrics takes priority over trailing hints while still shedding its own
   tokens on very narrow terminals.

### Data flow — `model.View()`

Set the new fields from model state when constructing `StatusBar`:

```go
sb.editorFocused = m.activePanel == editorPanel
sb.previewMode = m.editorMode == modePreview
sb.selectionActive = m.editor.selActive
sb.bufferText = m.editor.Value()
if m.editor.selActive {
    sb.selectionText = m.editor.SelectedText()
}
```

## Error handling

No new error paths. All helpers are pure and total over any string input
(including empty). `strings.Split` on an empty/edge buffer is handled by the
existing counting logic returning zero counts, which hides the segment.

## Testing — `metrics_test.go`

Counting (`computeMetrics`, `stripCodeFences`, `readingMinutes`):

- Plain prose: word and char counts for a known sentence.
- Code-fence stripping: fenced block excluded from counts; unterminated fence
  strips to end of buffer.
- Multi-byte characters: words and rune counts correct for non-ASCII text.
- Empty buffer and whitespace-only buffer: zero words.
- Reading-time boundaries: 0 words → 0; 1 word → 1m; 220 → 1m; 221 → 2m.

Segment selection (`StatusBar.metricTokens()`):

- Edit mode non-empty → `["N words", "C chars"]`, no prefix.
- Preview mode non-empty → `["~Rm read"]`.
- Selection active → `sel: ` prefix, words+chars, no reading time.
- Editor not focused → hidden.
- Empty note / empty selection → hidden.

## Out of scope

- Tilde (`~~~`) fenced blocks.
- Caching of computed counts.
- Configurable wpm.
