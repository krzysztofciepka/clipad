package main

import (
	"testing"
)

func TestMostRecentCitation_FindsLastAssistantTurnWithCites(t *testing.T) {
	turns := []chatTurn{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1", Citations: []citation{{Path: "old.md", StartLine: 1, EndLine: 1}}},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2", Citations: []citation{
			{Path: "new.md", StartLine: 5, EndLine: 7},
			{Path: "other.md", StartLine: 8, EndLine: 9},
		}},
	}
	c := mostRecentCitation(turns, 2)
	if c == nil || c.Path != "other.md" {
		t.Errorf("got %v, want other.md", c)
	}
	c = mostRecentCitation(turns, 1)
	if c == nil || c.Path != "new.md" {
		t.Errorf("got %v, want new.md", c)
	}
	if mostRecentCitation(turns, 99) != nil {
		t.Errorf("out-of-range should return nil")
	}
}

func TestMostRecentCitation_SkipsTurnsWithoutCites(t *testing.T) {
	turns := []chatTurn{
		{Role: "assistant", Content: "no cites"},
	}
	if mostRecentCitation(turns, 1) != nil {
		t.Errorf("should be nil when last assistant has no citations")
	}
}

func TestEncodeChatHistory_SingleTurnIsBareQuery(t *testing.T) {
	got := encodeChatHistory([]chatMessage{{Role: "user", Content: "what's my plan?"}})
	if got != "what's my plan?" {
		t.Errorf("got %q, want bare query", got)
	}
}

func TestEncodeChatHistory_MultiTurnIsTranscript(t *testing.T) {
	msgs := []chatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "current"},
	}
	got := encodeChatHistory(msgs)
	for _, want := range []string{"User: hello", "Assistant: hi", "User: current"} {
		if !contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestComposeChatRequest_StripsAssistantPlaceholder(t *testing.T) {
	turns := []chatTurn{
		{Role: "user", Content: "what is X?"},
		{Role: "assistant", Content: ""}, // UI placeholder
	}
	_, msgs, _ := composeChatRequest(turns, nil)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (the user query)", len(msgs))
	}
	if msgs[0].Content != "what is X?" {
		t.Errorf("user content = %q, want 'what is X?'", msgs[0].Content)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
