package main

import (
	"fmt"
	"strings"
)

const maxHistoryPairs = 4

type chatMessage struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// composeChatRequest builds the messages array for an Ask-your-vault call.
// turns is the full session history INCLUDING the user's brand-new question
// at the end. retrievedChunks is the top-K context for the current query.
func composeChatRequest(turns []chatTurn, retrievedChunks []Result) (system string, messages []chatMessage, citations []citation) {
	system = buildSystemPrompt(retrievedChunks)
	citations = make([]citation, len(retrievedChunks))
	for i, c := range retrievedChunks {
		citations[i] = citation{Path: c.Path, StartLine: c.StartLine, EndLine: c.EndLine}
	}

	if len(turns) == 0 || turns[len(turns)-1].Role != "user" {
		return system, nil, citations
	}
	currentUser := turns[len(turns)-1]
	prior := turns[:len(turns)-1]

	pairs := lastPairs(prior, maxHistoryPairs)
	for _, t := range pairs {
		messages = append(messages, chatMessage{Role: t.Role, Content: t.Content})
	}
	messages = append(messages, chatMessage{Role: "user", Content: currentUser.Content})
	return system, messages, citations
}

// lastPairs walks backwards collecting up to n complete (user, assistant) pairs.
func lastPairs(turns []chatTurn, n int) []chatTurn {
	var rev []chatTurn
	pairs := 0
	i := len(turns) - 1
	for i >= 0 && pairs < n {
		if turns[i].Role != "assistant" {
			break
		}
		assistant := turns[i]
		i--
		if i < 0 || turns[i].Role != "user" {
			break
		}
		user := turns[i]
		i--
		rev = append([]chatTurn{user, assistant}, rev...)
		pairs++
	}
	return rev
}

func buildSystemPrompt(chunks []Result) string {
	var b strings.Builder
	b.WriteString(`You are answering questions using the user's personal note vault as context.
Below are relevant excerpts. Cite sources inline using their numbered tag,
e.g., [1], [2]. If the excerpts do not contain the answer, say so plainly
rather than guessing.

`)
	for i, c := range chunks {
		fmt.Fprintf(&b, "[%d] %s L%d-L%d:\n%s\n\n", i+1, c.Path, c.StartLine, c.EndLine, c.Text)
	}
	return b.String()
}
