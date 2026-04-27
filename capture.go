package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
