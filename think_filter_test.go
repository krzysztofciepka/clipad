package main

import "testing"

// feedAll runs the chunks through a fresh filter and returns the concatenated
// emitted output plus the final flush.
func feedAll(chunks ...string) string {
	var f thinkFilter
	out := ""
	for _, c := range chunks {
		out += f.feed(c)
	}
	return out + f.flush()
}

func TestThinkFilter_NoThinkPassesThrough(t *testing.T) {
	if got := feedAll("hello ", "world"); got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestThinkFilter_StripsSingleBlock(t *testing.T) {
	if got := feedAll("<think>\nreasoning here\n</think>\n4"); got != "4" {
		t.Errorf("got %q, want %q", got, "4")
	}
}

func TestThinkFilter_TagSplitAcrossChunks(t *testing.T) {
	// open and close tags arrive split across chunk boundaries
	got := feedAll("<th", "ink>secret reas", "oning</thi", "nk>", "the answer")
	if got != "the answer" {
		t.Errorf("got %q, want %q", got, "the answer")
	}
}

func TestThinkFilter_LeadingWhitespaceTrimmedAfterThink(t *testing.T) {
	if got := feedAll("<think>x</think>\n\n  result"); got != "result" {
		t.Errorf("got %q, want %q", got, "result")
	}
}

func TestThinkFilter_LoneLessThanNotHeldForever(t *testing.T) {
	// a stray '<' that never becomes a tag must still be emitted at flush
	if got := feedAll("a < b"); got != "a < b" {
		t.Errorf("got %q, want %q", got, "a < b")
	}
}

func TestThinkFilter_TextBeforeThink(t *testing.T) {
	if got := feedAll("answer: ", "<think>nope</think>", "42"); got != "answer: 42" {
		t.Errorf("got %q, want %q", got, "answer: 42")
	}
}

func TestThinkFilter_UnclosedThinkDropped(t *testing.T) {
	// model cut off mid-thought: nothing emittable
	if got := feedAll("<think>still thinking when truncated"); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}
