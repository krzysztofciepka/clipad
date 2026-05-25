package main

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
