package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	shortcutItemStyle = lipgloss.NewStyle().
		PaddingLeft(1)

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

	shortcutSectionStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true).
		PaddingLeft(1)
)

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

// maxShortcutNameCol caps the name column so long fabric pattern names cannot
// squeeze the description column out of existence.
const maxShortcutNameCol = 30

// shortcutSelectorView renders the visible slice of picker rows. offset is the
// first row shown; the caller keeps the cursor inside the window via
// clampShortcutOffset.
func shortcutSelectorView(rows []shortcutRow, cursor, offset int, provider string, filtering bool, width, height int) string {
	box := lipgloss.NewStyle().
		Width(width).
		MaxHeight(height).
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	if len(rows) == 0 {
		msg := "No shortcuts. Press Ctrl+L to create one."
		if filtering {
			msg = "No matches."
		}
		return box.Height(height).Render(shortcutEmptyStyle.Render(msg))
	}

	maxName := 0
	for _, r := range rows {
		if r.kind == rowHeader {
			continue
		}
		if n := len([]rune(r.name)); n > maxName {
			maxName = n
		}
	}
	if maxName > maxShortcutNameCol {
		maxName = maxShortcutNameCol
	}
	nameCol := maxName + 2

	descBudget := width - 2 - 2 - nameCol - 3
	if descBudget < 0 {
		descBudget = 0
	}

	if offset < 0 || offset >= len(rows) {
		offset = 0
	}
	end := offset + visibleShortcutRows(height)
	if end > len(rows) {
		end = len(rows)
	}

	var out []string
	for i := offset; i < end; i++ {
		r := rows[i]
		if r.kind == rowHeader {
			out = append(out, shortcutSectionStyle.Render(r.name))
			continue
		}
		name := truncateRight(r.name, maxName)
		namePart := name + strings.Repeat(" ", nameCol-len([]rune(name)))
		var line string
		if i == cursor {
			line = shortcutCursorStyle.Render("> " + namePart)
		} else {
			line = shortcutItemStyle.Render("  " + namePart)
		}
		if r.description != "" {
			if desc := truncateRight(r.description, descBudget); desc != "" {
				line += shortcutDescStyle.Render("— " + desc)
			}
		}
		out = append(out, line)
	}

	providerLine := shortcutHintStyle.Render("Provider: " + provider + "  (p:cycle)")
	hint := shortcutHintStyle.Render("Enter:run  /:filter  e:edit  d:delete  Ctrl+↑/↓:reorder  Esc:close")
	if filtering {
		hint = shortcutHintStyle.Render("Enter:run  ↑/↓:move  Esc:clear filter")
	}
	return box.Render(strings.Join(out, "\n") + "\n" + providerLine + "\n" + hint)
}
