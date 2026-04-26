package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestComposeChatRequest_BoundsHistoryToFourPairs(t *testing.T) {
	var turns []chatTurn
	for i := 0; i < 8; i++ {
		turns = append(turns,
			chatTurn{Role: "user", Content: "u" + strconv.Itoa(i)},
			chatTurn{Role: "assistant", Content: "a" + strconv.Itoa(i)},
		)
	}
	turns = append(turns, chatTurn{Role: "user", Content: "current"})

	chunks := []Result{{Path: "x.md", StartLine: 1, EndLine: 1, Text: "hello"}}
	_, msgs, _ := composeChatRequest(turns, chunks)

	// Expected: 4 prior pairs (8 messages) + 1 current user = 9 total
	if len(msgs) != 9 {
		t.Errorf("messages = %d, want 9", len(msgs))
	}
	if msgs[len(msgs)-1].Content != "current" {
		t.Errorf("last message = %q, want 'current'", msgs[len(msgs)-1].Content)
	}
	if msgs[0].Content != "u4" {
		t.Errorf("first prior user = %q, want 'u4'", msgs[0].Content)
	}
}

func TestComposeChatRequest_SystemPromptHasCitationTags(t *testing.T) {
	chunks := []Result{
		{Path: "a.md", StartLine: 1, EndLine: 3, Text: "hello"},
		{Path: "b.md", StartLine: 5, EndLine: 7, Text: "world"},
	}
	turns := []chatTurn{{Role: "user", Content: "what?"}}
	sys, _, cites := composeChatRequest(turns, chunks)
	if !strings.Contains(sys, "[1] a.md L1-L3:") {
		t.Errorf("system missing [1] tag: %q", sys)
	}
	if !strings.Contains(sys, "[2] b.md L5-L7:") {
		t.Errorf("system missing [2] tag: %q", sys)
	}
	if len(cites) != 2 || cites[0].Path != "a.md" || cites[1].Path != "b.md" {
		t.Errorf("citations = %v", cites)
	}
}
