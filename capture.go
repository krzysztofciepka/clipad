package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// resolveInboxPath converts the raw config value into an absolute path
// to inbox.md, applying these rules:
//   - empty → "inbox.md" (relative)
//   - "~" prefix → home-dir expansion, treated as absolute
//   - filepath.IsAbs → used as-is, cleaned
//   - otherwise → joined with vault root
func resolveInboxPath(vault, configValue string) string {
	if configValue == "" {
		configValue = "inbox.md"
	}
	if strings.HasPrefix(configValue, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			configValue = filepath.Join(home, strings.TrimPrefix(configValue, "~"))
		}
	}
	if filepath.IsAbs(configValue) {
		return filepath.Clean(configValue)
	}
	return filepath.Join(vault, configValue)
}

// formatCaptureLine renders one inbox.md bullet for the given timestamp
// and capture text. Format: "- 2026-04-27 14:22 — <text>" (em-dash,
// minute precision, local time). Multi-line text embeds literal "\n"s;
// only the first line gets the bullet/timestamp prefix.
func formatCaptureLine(now time.Time, text string) string {
	return fmt.Sprintf("- %s — %s",
		now.Format("2006-01-02 15:04"),
		text)
}

// appendToInboxFile appends one bullet line to the given path, creating
// the file (and any missing parent dirs) if needed. Trailing-newline
// rules:
//   - the result always ends in exactly one "\n"
//   - if the existing file lacks a trailing "\n", one is inserted
//     before the new bullet (so we never produce "…texthello- ...")
//   - existing trailing blank lines (e.g. "\n\n") are preserved as-is
//     and the bullet is appended after them
func appendToInboxFile(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		b.WriteByte('\n')
	}
	b.WriteString(line)
	b.WriteByte('\n')
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ensureTrailingNewline returns s with a single "\n" at the end.
// Empty strings are returned unchanged (no spurious "\n").
func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

// writeNewFile writes content to path with O_CREATE|O_EXCL semantics:
// returns os.ErrExist if the file already exists. This is the atomic
// create-only primitive used by the delegate flow — it survives a
// TOCTOU race between the os.Stat collision check and the actual write.
func writeNewFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

var captureModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(0, 1)

// captureView renders the centered capture modal: a title line
// "Quick capture → <inbox path>" above the textarea contents.
// Caller is responsible for wrapping the result with lipgloss.Place
// to center it within the editor pane.
func captureView(textareaView, inboxPath string, screenWidth, screenHeight int) string {
	title := "Quick capture → " + inboxPath
	body := title + "\n\n" + textareaView + "\n\nEnter: save · Shift+Enter: newline · Esc: cancel"
	w := 60
	if screenWidth > 0 && w > screenWidth-4 {
		w = screenWidth - 4
	}
	return captureModalStyle.Width(w).Render(body)
}

// handleCapture handles key events while inputMode == inputCapture.
//
// Esc cancels (modal closes; underlying state untouched).
// Plain Enter submits — empty/whitespace-only input is a silent cancel.
// All other keys (including Shift+Enter for a newline) fall through
// to textarea.Update so the textarea handles them natively.
func (m model) handleCapture(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputNone
		m.captureInput.Blur()
		return m, nil

	case "enter":
		text := strings.TrimRight(m.captureInput.Value(), "\n")
		m.inputMode = inputNone
		m.captureInput.Blur()
		if strings.TrimSpace(text) == "" {
			return m, nil
		}
		return m.dispatchCapture(text)
	}

	var cmd tea.Cmd
	m.captureInput, cmd = m.captureInput.Update(msg)
	return m, cmd
}

// captureAppendedMsg is emitted by captureAppendCmd after the disk
// write completes (or fails). Carries enough info for the model to
// decide whether to reload an open editor view of inbox.md.
type captureAppendedMsg struct {
	err        error
	inboxPath  string
	reloadOpen bool
}

// captureAppendCmd performs the actual disk append off the main loop.
func captureAppendCmd(inboxPath, line string, reloadOpen bool) tea.Cmd {
	return func() tea.Msg {
		if err := appendToInboxFile(inboxPath, line); err != nil {
			return captureAppendedMsg{err: err}
		}
		return captureAppendedMsg{inboxPath: inboxPath, reloadOpen: reloadOpen}
	}
}

// appendLineToEditor appends a single bullet line to the in-memory
// editor buffer, ensuring the result is well-formed:
//   - existing buffer is given a trailing "\n" if it lacks one
//   - the new line is added with its own trailing "\n"
//   - the cursor's logical (row, col) is preserved across the
//     SetValue call (clamped to the new content bounds)
//
// The editor's existing isDirty mechanism flags the buffer as dirty
// automatically once SetValue runs.
func (m *model) appendLineToEditor(line string) {
	row, col := editorCursorPos(m.editor)

	old := m.editor.Value()
	var b strings.Builder
	b.WriteString(old)
	if len(old) > 0 && !strings.HasSuffix(old, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(line)
	b.WriteByte('\n')

	m.editor.SetValue(b.String())
	m.editor.MoveTo(row, col)
}

// handleDelegate handles key events while inputMode == inputDelegateName.
//
// Esc cancels (modal closes; selection on editor untouched).
// Empty Enter is ignored (user keeps typing).
// Filenames containing "/" or "\" are rejected — names only, target
// dir is fixed at the source file's parent directory.
// Filenames without an extension get ".md" auto-appended.
//
// On a valid name, the handler:
//  1. checks for collision against the target path
//  2. reads the editor's currently selected text
//  3. atomically writes the new file (writeNewFile / O_EXCL)
//  4. removes the selection from the editor (DeleteSelection)
//  5. saves the source via saveCurrentFile
//
// All other keys fall through to textinput.Update.
func (m model) handleDelegate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputNone
		m.delegateInput.Blur()
		return m, nil

	case "enter":
		raw := strings.TrimSpace(m.delegateInput.Value())
		if raw == "" {
			return m, nil // ignore; user keeps typing
		}
		name := raw
		if filepath.Ext(name) == "" {
			name += ".md"
		}
		if strings.ContainsAny(name, "/\\") {
			m.errMsg = "filename only — no slashes"
			return m, nil
		}
		// Happy path implemented in the next task.
		return m, nil
	}

	var cmd tea.Cmd
	m.delegateInput, cmd = m.delegateInput.Update(msg)
	return m, cmd
}

// dispatchCapture decides what to do with the captured text based on
// whether inbox.md is currently open in the editor and dirty.
//
// Branches:
//   - inbox not open → disk write only
//   - inbox open + clean → disk write + editor reload
//   - inbox open + dirty → in-memory editor append, no disk write
func (m model) dispatchCapture(text string) (tea.Model, tea.Cmd) {
	line := formatCaptureLine(time.Now(), text)
	inboxPath := resolveInboxPath(m.vault, m.inboxPath)

	if m.currentFile == inboxPath {
		if m.isDirty() {
			// Inbox is open with unsaved edits. Append in-memory
			// only; user keeps their dirty state and saves later
			// with Ctrl+S. The editor stays dirty automatically
			// because cleanContent is unchanged.
			m.appendLineToEditor(line)
			return m, nil
		}
		// Inbox is open and clean — disk write + reload editor on
		// completion (the captureAppendedMsg handler does the reload).
		return m, captureAppendCmd(inboxPath, line, true)
	}
	// Inbox is not open — just disk write; no reload needed.
	return m, captureAppendCmd(inboxPath, line, false)
}
