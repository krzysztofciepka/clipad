package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type searchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Score     float32
	Snippet   string
}

type vaultSearchResultsMsg struct {
	token   int64
	results []searchResult
	err     error
}

type vaultSearchTickMsg struct{ token int64 }

func vaultSearchTickCmd(token int64) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return vaultSearchTickMsg{token: token}
	})
}

// snippetFromText returns the first 2 wrapped lines of text, with tabs
// replaced by spaces, truncated to width chars per line.
func snippetFromText(text string, width int) string {
	if width < 1 {
		width = 1
	}
	t := strings.ReplaceAll(text, "\t", "    ")
	lines := strings.SplitN(t, "\n", 3)
	out := make([]string, 0, 2)
	for i, l := range lines {
		if i == 2 {
			break
		}
		if len(l) > width {
			l = l[:width-1] + "…"
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func toSearchResults(rs []Result, snippetWidth int) []searchResult {
	out := make([]searchResult, len(rs))
	for i, r := range rs {
		out[i] = searchResult{
			Path:      r.Path,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Score:     r.Score,
			Snippet:   snippetFromText(r.Text, snippetWidth),
		}
	}
	return out
}

func searchVaultCmd(idx *Index, token int64, query string, k, snippetWidth int) tea.Cmd {
	return func() tea.Msg {
		if idx == nil || idx.embedder == nil || strings.TrimSpace(query) == "" {
			return vaultSearchResultsMsg{token: token, results: nil}
		}
		ctx := context.Background()
		rs, err := idx.Search(ctx, query, k)
		if err != nil {
			return vaultSearchResultsMsg{token: token, err: err}
		}
		return vaultSearchResultsMsg{token: token, results: toSearchResults(rs, snippetWidth)}
	}
}

var (
	vaultSearchModalStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(1, 2)
	vaultSearchResultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	vaultSearchSelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("215")).Bold(true)
	vaultSearchPathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	vaultSearchScoreStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func formatLineRange(start, end int) string {
	if start == end {
		return "L" + strconv.Itoa(start)
	}
	return "L" + strconv.Itoa(start) + "-L" + strconv.Itoa(end)
}

func formatScore(s float32) string {
	return fmt.Sprintf("(%.2f)", s)
}

// vaultSearchView renders the modal. screenWidth/Height are the full window
// dimensions; the modal is sized as a fraction of those.
func vaultSearchView(input string, results []searchResult, cursor int, offset int, screenWidth, screenHeight int) string {
	w := screenWidth * 4 / 5
	if w < 50 {
		w = 50
	}
	h := screenHeight * 7 / 10
	if h < 12 {
		h = 12
	}

	var b strings.Builder
	b.WriteString(vaultSearchPathStyle.Render("Vault Search") + "  " + vaultSearchScoreStyle.Render("(Esc to close)"))
	b.WriteString("\n\n> " + input + "\n\n")

	visibleSlots := h - 6
	if cursor >= offset+visibleSlots/3 {
		offset = cursor - visibleSlots/3
		if offset < 0 {
			offset = 0
		}
	}
	for i := offset; i < len(results); i++ {
		r := results[i]
		header := vaultSearchPathStyle.Render(r.Path) +
			"  " + vaultSearchScoreStyle.Render(formatLineRange(r.StartLine, r.EndLine)) +
			"  " + vaultSearchScoreStyle.Render(formatScore(r.Score))
		marker := "  "
		style := vaultSearchResultStyle
		if i == cursor {
			marker = vaultSearchSelStyle.Render("❯ ")
			style = vaultSearchSelStyle
		}
		b.WriteString(marker + header + "\n")
		for _, line := range strings.Split(r.Snippet, "\n") {
			b.WriteString("    " + style.Render(line) + "\n")
		}
		b.WriteString("\n")
	}
	if len(results) == 0 && strings.TrimSpace(input) != "" {
		b.WriteString(vaultSearchScoreStyle.Render("(no results)\n"))
	}

	footer := vaultSearchScoreStyle.Render("Enter: open · ↑↓: navigate · Esc: close")
	b.WriteString("\n" + footer)

	return vaultSearchModalStyle.Width(w).Height(h).Render(b.String())
}
