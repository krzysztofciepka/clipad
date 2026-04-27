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

// dispatchCapture is filled in by Task 10. Stub returns no-op so
// handleCapture compiles.
func (m model) dispatchCapture(text string) (tea.Model, tea.Cmd) {
	return m, nil
}
