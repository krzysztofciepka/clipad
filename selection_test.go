package main

import (
	"testing"
)

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true}, {'Z', true}, {'0', true}, {'_', true},
		{' ', false}, {'.', false}, {'-', false}, {'\n', false},
	}
	for _, tt := range tests {
		if got := isWordChar(tt.r); got != tt.want {
			t.Errorf("isWordChar(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestWordLeftPos(t *testing.T) {
	tests := []struct {
		content  string
		line     int
		col      int
		wantLine int
		wantCol  int
	}{
		{"hello world", 0, 11, 0, 6},
		{"hello world", 0, 6, 0, 0},
		{"hello world", 0, 8, 0, 6},
		{"hello world", 0, 0, 0, 0},
		{"first\nsecond", 1, 0, 0, 0},
		{"hello  world", 0, 7, 0, 0},
	}
	for _, tt := range tests {
		gotLine, gotCol := wordLeftPos(tt.content, tt.line, tt.col)
		if gotLine != tt.wantLine || gotCol != tt.wantCol {
			t.Errorf("wordLeftPos(%q, %d, %d) = (%d, %d), want (%d, %d)",
				tt.content, tt.line, tt.col, gotLine, gotCol, tt.wantLine, tt.wantCol)
		}
	}
}

func TestWordRightPos(t *testing.T) {
	tests := []struct {
		content  string
		line     int
		col      int
		wantLine int
		wantCol  int
	}{
		{"hello world", 0, 0, 0, 6},
		{"hello world", 0, 6, 0, 11},
		{"hello world", 0, 3, 0, 6},
		{"first\nsecond", 0, 0, 0, 5},
		{"first\nsecond", 0, 5, 1, 0},
	}
	for _, tt := range tests {
		gotLine, gotCol := wordRightPos(tt.content, tt.line, tt.col)
		if gotLine != tt.wantLine || gotCol != tt.wantCol {
			t.Errorf("wordRightPos(%q, %d, %d) = (%d, %d), want (%d, %d)",
				tt.content, tt.line, tt.col, gotLine, gotCol, tt.wantLine, tt.wantCol)
		}
	}
}

func TestSelectionRange(t *testing.T) {
	sL, sC, eL, eC := selectionRange(0, 0, 1, 5)
	if sL != 0 || sC != 0 || eL != 1 || eC != 5 {
		t.Errorf("selectionRange(0,0,1,5) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
	sL, sC, eL, eC = selectionRange(1, 5, 0, 0)
	if sL != 0 || sC != 0 || eL != 1 || eC != 5 {
		t.Errorf("selectionRange(1,5,0,0) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
	sL, sC, eL, eC = selectionRange(2, 10, 2, 3)
	if sL != 2 || sC != 3 || eL != 2 || eC != 10 {
		t.Errorf("selectionRange(2,10,2,3) = (%d,%d,%d,%d)", sL, sC, eL, eC)
	}
}

func TestExtractText(t *testing.T) {
	content := "hello world\nfoo bar\nbaz"
	tests := []struct {
		sLine, sCol, eLine, eCol int
		want                     string
	}{
		{0, 0, 0, 5, "hello"},
		{0, 6, 0, 11, "world"},
		{0, 0, 1, 3, "hello world\nfoo"},
		{0, 0, 2, 3, "hello world\nfoo bar\nbaz"},
		{1, 4, 2, 3, "bar\nbaz"},
	}
	for _, tt := range tests {
		got := extractText(content, tt.sLine, tt.sCol, tt.eLine, tt.eCol)
		if got != tt.want {
			t.Errorf("extractText(%d,%d,%d,%d) = %q, want %q",
				tt.sLine, tt.sCol, tt.eLine, tt.eCol, got, tt.want)
		}
	}
}

func TestDeleteText(t *testing.T) {
	content := "hello world\nfoo bar\nbaz"
	tests := []struct {
		sLine, sCol, eLine, eCol int
		want                     string
	}{
		{0, 5, 0, 11, "hello\nfoo bar\nbaz"},
		{0, 0, 0, 5, " world\nfoo bar\nbaz"},
		{0, 5, 1, 4, "hellobar\nbaz"},
		{0, 0, 2, 3, ""},
	}
	for _, tt := range tests {
		got := deleteText(content, tt.sLine, tt.sCol, tt.eLine, tt.eCol)
		if got != tt.want {
			t.Errorf("deleteText(%d,%d,%d,%d) = %q, want %q",
				tt.sLine, tt.sCol, tt.eLine, tt.eCol, got, tt.want)
		}
	}
}

func TestPosInRange(t *testing.T) {
	tests := []struct {
		line, col int
		want      bool
	}{
		{0, 0, false}, {0, 1, false}, {0, 2, true}, {0, 5, true},
		{1, 0, true}, {1, 2, true}, {1, 3, false}, {1, 4, false},
		{2, 0, false},
	}
	for _, tt := range tests {
		got := posInRange(tt.line, tt.col, 0, 2, 1, 3)
		if got != tt.want {
			t.Errorf("posInRange(%d, %d, 0,2,1,3) = %v, want %v", tt.line, tt.col, got, tt.want)
		}
	}
}
