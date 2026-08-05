package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type chatModeT int

const (
	chatModeInput chatModeT = iota
	chatModeView
)

type chatTurn struct {
	Role      string // "user" | "assistant"
	Content   string
	Citations []citation
	Trace     []traceLine // tool activity for assistant turns (in order)
}

type traceLine struct {
	Kind string // "cmd" | "result" | "search"
	Text string
	OK   bool
}

type citation struct {
	Path      string
	StartLine int
	EndLine   int
}

var (
	chatPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
	chatUserStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("215")).Bold(true)
	chatAssistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	chatCitationStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	chatHintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func renderChatScrollback(turns []chatTurn, width int, streaming bool) string {
	if width < 10 {
		width = 10
	}
	var b strings.Builder
	for i, t := range turns {
		if i > 0 {
			b.WriteString("\n")
		}
		switch t.Role {
		case "user":
			body := wordWrap("You: "+t.Content, width)
			b.WriteString(chatUserStyle.Render("▸ ") + body + "\n")
		case "assistant":
			for _, tl := range t.Trace {
				style := chatHintStyle
				if tl.Kind == "result" && !tl.OK {
					style = chatUserStyle
				}
				b.WriteString("  " + style.Render(wordWrap(tl.Text, width-2)) + "\n")
			}
			content := t.Content
			// While streaming, show a placeholder for the empty in-flight turn.
			if content == "" && len(t.Trace) == 0 && streaming && i == len(turns)-1 {
				content = "(thinking…)"
			}
			if content != "" {
				body := wordWrap("clipad: "+content, width)
				b.WriteString(chatAssistantStyle.Render(body) + "\n")
			}
			for j, c := range t.Citations {
				cite := fmt.Sprintf("[%d] %s L%d-L%d", j+1, c.Path, c.StartLine, c.EndLine)
				b.WriteString("  " + chatCitationStyle.Render(wordWrap(cite, width-2)) + "\n")
			}
		}
	}
	return b.String()
}

func chatPanelView(vp viewport.Model, input string, mode chatModeT, width, height int) string {
	hint := "1-9: open citation · i: input · ↑↓: scroll · Esc: close"
	if mode == chatModeInput {
		hint = "Enter: send · Esc: view mode"
	}
	body := vp.View()
	footer := chatHintStyle.Render(hint)
	inputLine := "> " + input
	return chatPanelStyle.Width(width).Height(height).Render(body + "\n" + inputLine + "\n" + footer)
}

// chatCitations returns the citations of the most recent assistant turn that
// has any. The numbered 1–9 shortcuts and the Prev/Next cite buttons both
// index into this list.
func chatCitations(turns []chatTurn) []citation {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "assistant" && len(turns[i].Citations) > 0 {
			return turns[i].Citations
		}
	}
	return nil
}

// nextCiteIndex advances a 1-based citation cursor by delta, wrapping at both
// ends. A cursor of 0 means "nothing selected yet": stepping forward lands on
// the first citation, stepping back on the last.
func nextCiteIndex(cursor, delta, n int) int {
	if n <= 0 {
		return 0
	}
	if cursor == 0 {
		if delta > 0 {
			return 1
		}
		return n
	}
	i := (cursor - 1 + delta) % n
	if i < 0 {
		i += n
	}
	return i + 1
}

// mostRecentCitation returns the Nth citation (1-indexed) of the most recent
// assistant turn that has citations, or nil if there isn't one.
func mostRecentCitation(turns []chatTurn, n int) *citation {
	cites := chatCitations(turns)
	if n < 1 || n > len(cites) {
		return nil
	}
	c := cites[n-1]
	return &c
}
