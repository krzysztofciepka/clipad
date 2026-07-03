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
