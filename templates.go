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

// templateVarRe matches {{name}} and {{name:layout}} for the supported variable
// names only. Unknown placeholders never match and pass through untouched. The
// optional :layout is only honored for {{date}}; on other names it is ignored.
// The layout cannot contain braces (Go reference layouts never do).
var templateVarRe = regexp.MustCompile(`\{\{(date|time|yesterday|vault)(?::([^{}]*))?\}\}`)

// renderTemplate substitutes template variables in content. now is injected so
// rendering is deterministic and unit-testable.
func renderTemplate(content string, now time.Time, vault string) string {
	return templateVarRe.ReplaceAllStringFunc(content, func(match string) string {
		// ReplaceAllStringFunc only passes the whole match, so re-parse it here
		// to recover the name and optional layout capture groups.
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

// startTemplatePicker seeds the default, loads the template list, captures the
// target directory from the current selection, and opens the picker modal.
func (m *model) startTemplatePicker() {
	if err := seedDefaultTemplate(); err != nil {
		m.errMsg = fmt.Sprintf("Template setup failed: %v", err)
		return
	}
	names, err := listTemplates()
	if err != nil {
		m.errMsg = fmt.Sprintf("Listing templates failed: %v", err)
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

// openDailyNote opens <vault>/daily/YYYY-MM-DD.md, creating it from the daily
// template when absent. An existing note is opened as-is (not re-rendered).
func (m *model) openDailyNote() {
	if err := seedDefaultTemplate(); err != nil {
		m.errMsg = fmt.Sprintf("Template setup failed: %v", err)
		return
	}
	now := time.Now()
	dailyDir := filepath.Join(m.vault, "daily")
	path := filepath.Join(dailyDir, now.Format("2006-01-02")+".md")

	if _, statErr := os.Stat(path); statErr == nil {
		m.openFile(path)
		m.activePanel = editorPanel
		m.editor.Focus()
		return
	} else if !os.IsNotExist(statErr) {
		m.errMsg = fmt.Sprintf("Open failed: %v", statErr)
		return
	}

	tmpl, err := os.ReadFile(filepath.Join(templatesDir(), "daily.md"))
	if err != nil {
		m.errMsg = fmt.Sprintf("Read template failed: %v", err)
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
