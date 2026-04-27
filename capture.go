package main

import (
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
