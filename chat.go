package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

// chatChunkMsg / chatDoneMsg / chatErrMsg mirror the plugin streaming msgs
// but with their own identity discriminator so the two flows don't collide.
type chatChunkMsg struct {
	chunks <-chan string
	errs   <-chan error
	delta  string
}
type chatDoneMsg struct{ chunks <-chan string }
type chatErrMsg struct {
	chunks <-chan string
	err    error
}

type chatStartedMsg struct {
	chunks    <-chan string
	errs      <-chan error
	citations []citation
}
type chatStartFailedMsg struct{ err error }

func streamChatCmd(chunks <-chan string, errs <-chan error) tea.Cmd {
	return readNextChatChunk(chunks, errs)
}

func readNextChatChunk(chunks <-chan string, errs <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case d, ok := <-chunks:
			if !ok {
				return chatDoneMsg{chunks: chunks}
			}
			return chatChunkMsg{chunks: chunks, errs: errs, delta: d}
		case err := <-errs:
			if err != nil {
				return chatErrMsg{chunks: chunks, err: err}
			}
			return chatDoneMsg{chunks: chunks}
		}
	}
}

// chatStartCmd performs retrieval, composes the request, and starts the stream.
// It expects turns to already include the new user turn.
func chatStartCmd(idx *Index, turns []chatTurn, query string, providerURL, apiKey, chatModel string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var chunks []Result
		if idx != nil {
			rs, err := idx.Search(ctx, query, 8)
			if err != nil {
				return chatStartFailedMsg{err: fmt.Errorf("retrieval: %w", err)}
			}
			chunks = rs
		}
		sys, msgs, cites := composeChatRequest(turns, chunks)
		userMsg := encodeChatHistory(msgs)
		ch, errsCh := streamChatCompletion(ctx, providerURL, apiKey, chatModel, sys, userMsg)
		return chatStartedMsg{chunks: ch, errs: errsCh, citations: cites}
	}
}

// encodeChatHistory packs prior turns + current user into a single string for
// streamChatCompletion's userMessage parameter (which only takes system + user).
// The retrieved-chunk context lives in the system prompt.
//
// For a single-turn message (no prior history) the output is just the user's
// query verbatim. For multi-turn, prior turns render as a "User: …" /
// "Assistant: …" transcript, with the current question as the last entry.
func encodeChatHistory(msgs []chatMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	if len(msgs) == 1 && msgs[0].Role == "user" {
		return msgs[0].Content
	}
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch m.Role {
		case "user":
			b.WriteString("User: ")
		case "assistant":
			b.WriteString("Assistant: ")
		}
		b.WriteString(m.Content)
	}
	return b.String()
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

// mostRecentCitation returns the Nth citation (1-indexed) of the most recent
// assistant turn that has citations, or nil if there isn't one.
func mostRecentCitation(turns []chatTurn, n int) *citation {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Role == "assistant" && len(turns[i].Citations) > 0 {
			if n >= 1 && n <= len(turns[i].Citations) {
				c := turns[i].Citations[n-1]
				return &c
			}
			return nil
		}
	}
	return nil
}
