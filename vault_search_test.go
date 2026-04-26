package main

import (
	"strings"
	"testing"
)

func TestSnippetFromText_Truncates(t *testing.T) {
	in := "line one is quite a bit longer than the width\nline two short\nline three is dropped"
	got := snippetFromText(in, 30)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.HasSuffix(lines[0], "…") {
		t.Errorf("line 0 should be truncated: %q", lines[0])
	}
	if lines[1] != "line two short" {
		t.Errorf("line 1 = %q", lines[1])
	}
}

func TestSnippetFromText_PreservesShort(t *testing.T) {
	got := snippetFromText("hi", 30)
	if got != "hi" {
		t.Errorf("got %q", got)
	}
}

func TestFormatLineRange(t *testing.T) {
	if got := formatLineRange(5, 5); got != "L5" {
		t.Errorf("got %q, want L5", got)
	}
	if got := formatLineRange(5, 10); got != "L5-L10" {
		t.Errorf("got %q, want L5-L10", got)
	}
}
