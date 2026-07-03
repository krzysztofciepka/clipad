package main

import "testing"

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
