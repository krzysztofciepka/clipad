package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestChunkFile_SimpleParagraphs(t *testing.T) {
	in := "First paragraph line one.\nFirst paragraph line two.\n\nSecond paragraph.\n\nThird."
	got := chunkFile(in)
	want := []chunk{
		{StartLine: 1, EndLine: 2, Text: "First paragraph line one.\nFirst paragraph line two."},
		{StartLine: 4, EndLine: 4, Text: "Second paragraph."},
		{StartLine: 6, EndLine: 6, Text: "Third."},
	}
	for i := range want {
		want[i].Hash = chunkHash(want[i].Text)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chunkFile got %#v\nwant %#v", got, want)
	}
}

func TestChunkFile_Empty(t *testing.T) {
	if got := chunkFile(""); len(got) != 0 {
		t.Errorf("chunkFile(\"\") = %v, want empty", got)
	}
}

func TestChunkFile_WhitespaceOnly(t *testing.T) {
	if got := chunkFile("   \n\n\t\n"); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestChunkFile_OversizeParagraphSplits(t *testing.T) {
	line := strings.Repeat("a ", 300) // 600 chars
	para := line + "\n" + line + "\n" + line + "\n" + line + "\n" + line
	got := chunkFile(para)
	if len(got) < 2 {
		t.Fatalf("expected oversize paragraph to split into >=2 chunks, got %d", len(got))
	}
	for _, c := range got {
		if len(c.Text) > maxChunkChars+len(line) {
			t.Errorf("chunk too large: %d chars", len(c.Text))
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i].StartLine != got[i-1].EndLine+1 {
			t.Errorf("line gap: chunk[%d] starts at %d, prev ended at %d", i, got[i].StartLine, got[i-1].EndLine)
		}
	}
}
