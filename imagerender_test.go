package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/kitty"
)

func TestDetectKittyGraphics(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if !detectKittyGraphics(env(map[string]string{"KITTY_WINDOW_ID": "1"})) {
		t.Error("kitty window id should enable")
	}
	if !detectKittyGraphics(env(map[string]string{"TERM": "xterm-kitty"})) {
		t.Error("xterm-kitty TERM should enable")
	}
	if !detectKittyGraphics(env(map[string]string{"TERM_PROGRAM": "ghostty"})) {
		t.Error("ghostty should enable")
	}
	if !detectKittyGraphics(env(map[string]string{"TERM_PROGRAM": "WezTerm"})) {
		t.Error("wezterm should enable")
	}
	if detectKittyGraphics(env(map[string]string{"TERM": "xterm-256color"})) {
		t.Error("plain xterm should not enable")
	}
}

func TestImageRenderSize(t *testing.T) {
	// 800x400 image, 8x16 cells => 100 cols x 25 rows natural; clamp to 64x12.
	cols, rows := imageRenderSize(800, 400, 8, 16, 64, 12)
	if cols < 1 || rows < 1 {
		t.Fatalf("got %dx%d, want >=1", cols, rows)
	}
	if cols > 64 || rows > 12 {
		t.Errorf("not clamped: %dx%d", cols, rows)
	}
	// Aspect ratio preserved within rounding: natural cols/rows ~ 4:1.
	if rows != 12 && cols != 64 {
		t.Errorf("expected one dimension at the cap, got %dx%d", cols, rows)
	}
	// Tiny image stays tiny and never zero.
	c2, r2 := imageRenderSize(4, 8, 8, 16, 64, 12)
	if c2 < 1 || r2 < 1 {
		t.Errorf("tiny image => %dx%d, want >=1", c2, r2)
	}
}

func TestBuildPlaceholderBlock(t *testing.T) {
	rows := buildPlaceholderBlock(1, 2, 1)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	line := rows[0]
	// id=1 => 24-bit fg 0;0;1
	if !strings.HasPrefix(line, "\x1b[38;2;0;0;1m") {
		t.Errorf("missing fg color prefix: %q", line)
	}
	if !strings.HasSuffix(line, "\x1b[39m") {
		t.Errorf("missing fg reset suffix: %q", line)
	}
	if strings.Count(line, string(kitty.Placeholder)) != 2 {
		t.Errorf("want 2 placeholder cells, got %d", strings.Count(line, string(kitty.Placeholder)))
	}
	// First cell: placeholder + row diacritic(0) + col diacritic(0).
	firstCell := string(kitty.Placeholder) + string(kitty.Diacritic(0)) + string(kitty.Diacritic(0))
	if !strings.Contains(line, firstCell) {
		t.Errorf("first cell encoding missing in %q", line)
	}
}

func TestBuildTransmitSequenceSmall(t *testing.T) {
	seq := buildTransmitSequence(7, 3, 2, []byte("PNG"))
	if !strings.HasPrefix(seq, "\x1b_G") {
		t.Errorf("not an APC _G sequence: %q", seq)
	}
	for _, want := range []string{"i=7", "U=1", "a=T", "f=100", "c=3", "r=2"} {
		if !strings.Contains(seq, want) {
			t.Errorf("transmit seq missing %q: %q", want, seq)
		}
	}
	if !strings.HasSuffix(seq, "\x1b\\") {
		t.Errorf("not terminated with ST: %q", seq)
	}
}

func TestImageRegistry(t *testing.T) {
	r := newImageRegistry()
	id1 := r.idFor("aaa")
	id2 := r.idFor("bbb")
	if id1 == id2 || id1 < 1 || id2 < 1 {
		t.Errorf("ids not distinct/positive: %d %d", id1, id2)
	}
	if r.idFor("aaa") != id1 {
		t.Error("id not stable for same hash")
	}
	if !r.markTransmitted("aaa") {
		t.Error("first markTransmitted should be true")
	}
	if r.markTransmitted("aaa") {
		t.Error("second markTransmitted should be false")
	}
}
